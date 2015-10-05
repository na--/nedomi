package cache

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/ironsmile/nedomi/storage"
	"github.com/ironsmile/nedomi/types"
	"github.com/ironsmile/nedomi/utils"
	"github.com/ironsmile/nedomi/utils/cacheutils"
	"github.com/ironsmile/nedomi/utils/httputils"
)

// Hop-by-hop headers. These are removed when sent to the client.
// http://www.w3.org/Protocols/rfc2616/rfc2616-sec13.html
var hopHeaders = []string{
	"Connection",
	"Keep-Alive",
	"Proxy-Authenticate",
	"Proxy-Authorization",
	"Te", // canonicalized version of "TE"
	"Trailers",
	"Transfer-Encoding",
	"Upgrade",
}

//!TODO: add Date and cache-expity headers here? we probably have to manage them on our own
var metadataHeadersToFilter = append(hopHeaders, "Content-Length", "Content-Range")

// Returns a new HTTP 1.1 request that has no body. It also clears headers like
// accept-encoding and rearranges the requested ranges so they match part
func (h *reqHandler) getNormalizedRequest() *http.Request {
	url := *h.req.URL
	result := &http.Request{
		Method:     h.req.Method,
		URL:        &url,
		Proto:      "HTTP/1.1",
		ProtoMajor: 1,
		ProtoMinor: 1,
		Header:     make(http.Header),
		Host:       h.req.URL.Host,
	}

	httputils.CopyHeadersWithout(h.req.Header, result.Header, "Accept-Encoding")

	//!TODO: fix requested range to be divisible by the storage partSize

	return result
}

func (h *reqHandler) getResponseHook() func(*httputils.FlexibleResponseWriter) {

	return func(rw *httputils.FlexibleResponseWriter) {
		h.Logger.Debugf("[%p] Received headers for %s, sending them to client...", h.req, h.req.URL)
		httputils.CopyHeadersWithout(rw.Headers, h.resp.Header(), hopHeaders...)
		h.resp.WriteHeader(rw.Code)

		isCacheable := cacheutils.IsResponseCacheable(rw.Code, rw.Headers)
		if !isCacheable {
			h.Logger.Debugf("[%p] Response is non-cacheable", h.req)
			rw.BodyWriter = h.resp
			return
		}

		expiresIn := cacheutils.ResponseExpiresIn(rw.Headers, h.CacheDefaultDuration)
		if expiresIn <= 0 {
			h.Logger.Debugf("[%p] Response expires in the past: %s", h.req, expiresIn)
			rw.BodyWriter = h.resp
			return
		}

		responseRange, err := httputils.GetResponseRange(rw.Code, rw.Headers)
		if err != nil {
			h.Logger.Debugf("[%p] Was not able to get response range (%s)", h.req, err)
			rw.BodyWriter = h.resp
			return
		}

		h.Logger.Debugf("[%p] Response is cacheable! Caching metadata and parts...", h.req)

		code := rw.Code
		if code == http.StatusPartialContent {
			// 206 is returned only if the server would have returned 200 with a normal request
			code = http.StatusOK
		}

		//!TODO: maybe call cached time.Now. See the comment in utils.IsMetadataFresh
		now := time.Now()

		obj := &types.ObjectMetadata{
			ID:                h.objID,
			ResponseTimestamp: now.Unix(),
			Code:              code,
			Size:              responseRange.ObjSize,
			Headers:           make(http.Header),
			ExpiresAt:         now.Add(expiresIn).Unix(),
		}
		httputils.CopyHeadersWithout(rw.Headers, obj.Headers, metadataHeadersToFilter...)

		//!TODO: consult the cache algorithm whether to save the metadata
		//!TODO: optimize this, save the metadata only when it's newer
		//!TODO: also, error if we already have fresh metadata but the received metadata is different
		if err := h.Cache.Storage.SaveMetadata(obj); err != nil {
			h.Logger.Errorf("[%p] Could not save metadata for %s: %s", h.req, obj.ID, err)
			rw.BodyWriter = h.resp
			return
		}

		if h.req.Method == "HEAD" {
			rw.BodyWriter = h.resp
			return
		}

		rw.BodyWriter = utils.MultiWriteCloser(
			h.resp,
			PartWriter(h.Cache, h.objID, *responseRange),
		)

		h.Logger.Debugf("[%p] Setting the cached data to expire in %s", h.req, expiresIn)
		h.Cache.Scheduler.AddEvent(
			h.objID.Hash(),
			storage.GetExpirationHandler(h.Cache, h.Logger, h.objID),
			expiresIn,
		)
	}
}

func (h *reqHandler) getUpstreamReader(start, end uint64) io.ReadCloser {
	subh := *h
	subh.req = subh.getNormalizedRequest()
	subh.req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", start, end))

	h.Logger.Debugf("[%p] Making upstream request for %s, bytes [%d-%d]...",
		subh.req, subh.req.URL, start, end)

	//!TODO: optimize requests for the same pieces? if possible, make only 1 request to the upstream for the same part

	r, w := io.Pipe()
	subh.resp = httputils.NewFlexibleResponseWriter(func(rw *httputils.FlexibleResponseWriter) {
		respRng, err := httputils.GetResponseRange(rw.Code, rw.Headers)
		if err != nil {
			h.Logger.Errorf("[%p] Could not parse the content-range for the partial upstream request: %s", subh.req, err)
			_ = w.CloseWithError(err)
		}
		h.Logger.Debugf("[%p] Received response with status %d and range %v", subh.req, rw.Code, respRng)
		if rw.Code == http.StatusPartialContent {
			//!TODO: check whether the returned range corresponds to the requested range
			rw.BodyWriter = w
		} else if rw.Code == http.StatusOK {
			//!TODO: handle this, use skipWriter or something like that
			_ = w.CloseWithError(fmt.Errorf("NOT IMPLEMENTED"))
		} else {
			_ = w.CloseWithError(fmt.Errorf("Upstream responded with status %d", rw.Code))
		}
	})
	go subh.carbonCopyProxy()
	return r
}

func (h *reqHandler) getSmartReader(start, end uint64) io.ReadCloser {

	partSize := h.Cache.Storage.PartSize()
	localCount := 0
	indexes := utils.BreakInIndexes(h.objID, start, end, partSize)
	var lastPresentIndex *types.ObjectIndex
	readers := []io.ReadCloser{}

	h.Logger.Debugf("[%p] Trying to load all possible parts of %s from storage...", h.req, h.objID)
	for notAnIndexIndex, partIndex := range indexes {
		cached := h.Cache.Algorithm.Lookup(partIndex)
		r, err := h.Cache.Storage.GetPart(partIndex)
		if err != nil {
			if !os.IsNotExist(err) {
				h.Logger.Errorf("[%p] Unexpected error while trying to load %s from storage: %s", h.req, partIndex, err)
			}
			if cached {
				h.Logger.Errorf("[%p] Cache.Algorithm said a part %s is cached but Storage couldn't find it ", h.req, partIndex)
			}
			continue
		}
		h.Cache.Algorithm.PromoteObject(partIndex)

		if (lastPresentIndex == nil && notAnIndexIndex != 0) || (lastPresentIndex != nil && lastPresentIndex.Part != partIndex.Part-1) {
			fromPart := uint64(indexes[0].Part)
			if lastPresentIndex != nil {
				fromPart = uint64(lastPresentIndex.Part) + 1
			}
			toPart := uint64(partIndex.Part - 1)
			h.Logger.Debugf("[%p] Getting parts [%d-%d] from upstream!", h.req, fromPart, toPart)
			readers = append(readers, h.getUpstreamReader(fromPart*partSize, (toPart+1)*partSize-1))
		}
		h.Logger.Debugf("[%p] Loaded part %s from storage!", h.req, partIndex)
		localCount++
		readers = append(readers, r)
		lastPresentIndex = partIndex
	}
	// work in start and end
	var startOffset, endLimit = start % partSize, end%partSize + 1

	if lastPresentIndex != indexes[len(indexes)-1] {
		fromPart := uint64(indexes[0].Part)
		if lastPresentIndex != nil {
			fromPart = uint64(lastPresentIndex.Part + 1)
		}
		h.Logger.Debugf("[%p] Getting parts [%d-%d] from upstream!", h.req, fromPart, indexes[len(indexes)-1].Part)
		readers = append(readers, h.getUpstreamReader(fromPart*partSize, end))
	} else {
		readers[len(readers)-1] = utils.LimitReadCloser(readers[len(readers)-1], int(endLimit))
	}

	readers[0] = utils.SkipReadCloser(readers[0], int64(startOffset))
	h.Logger.Debugf("[%p] Return smart reader for %s with %d out of %d parts from storage!",
		h.req, h.objID, localCount, len(indexes))
	return utils.MultiReadCloser(readers...)
}