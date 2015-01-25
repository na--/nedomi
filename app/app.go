package app

import (
	"errors"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/gophergala/nedomi/config"
)

/*
   Application is the type which represents the webserver. It is responsible for
   parsing the config and it has Start, Stop, Reload and Wait functions.
*/
type Application struct {
	cfg *config.Config

	handlerWg sync.WaitGroup

	// The http handler for the main server loop
	httpHandler http.Handler

	// The listener for the main server loop
	listener net.Listener

	// HTTP Server which will use the above listener in order to server
	// clients requests.
	httpSrv *http.Server
}

/*
   Start fires up the application.
*/
func (a *Application) Start() error {
	if a.cfg == nil {
		return errors.New("Cannot start application with emtpy config")
	}

	startError := make(chan error)

	a.handlerWg.Add(1)
	go a.doServing(startError)

	if err := <-startError; err != nil {
		return err
	}

	log.Printf("Application %d started\n", os.Getpid())

	return nil
}

/*
   This routine actually starts listening and working on clients requests.
*/
func (a *Application) doServing(startErrChan chan<- error) {
	defer a.handlerWg.Done()

	a.httpHandler = newProxyHandler(a.cfg.HTTP)

	a.httpSrv = &http.Server{
		Addr:           a.cfg.HTTP.Listen,
		Handler:        a.httpHandler,
		ReadTimeout:    time.Duration(a.cfg.HTTP.ReadTimeout) * time.Second,
		WriteTimeout:   time.Duration(a.cfg.HTTP.WriteTimeout) * time.Second,
		MaxHeaderBytes: a.cfg.HTTP.MaxHeadersSize,
	}

	err := a.listenAndServe(startErrChan)

	log.Printf("Webserver stopped. %s", err)
}

// Uses our own listener to make our server stoppable. Similar to
// net.http.Server.ListenAndServer only this version saves a reference to the listener
func (a *Application) listenAndServe(startErrChan chan<- error) error {
	addr := a.httpSrv.Addr
	if addr == "" {
		addr = ":http"
	}
	lsn, err := net.Listen("tcp", addr)
	if err != nil {
		startErrChan <- err
		return err
	}
	a.listener = lsn
	startErrChan <- nil
	log.Println("Webserver started.")
	return a.httpSrv.Serve(lsn)
}

/*
   Stop makes sure the application is completely stopped and all of its
   goroutines and channels are finished and closed.
*/
func (a *Application) Stop() error {
	a.listener.Close()
	a.handlerWg.Wait()
	return nil
}

/*
   Reload takse a new configuration and replaces the old one with it. After succesful
   reload the things that are written in the new config will be in use.
*/
func (a *Application) Reload(cfg *config.Config) error {
	if cfg == nil {
		return errors.New("Config for realoding was nil. Reloading aborted.")
	}
	//!TODO: save the listnening handler if needed
	if err := a.Stop(); err != nil {
		return err
	}
	a.cfg = cfg
	return a.Start()
}

/*
   Wait subscribes iteself to few signals and waits for any of them to be received.
   When Wait returns it is the end of the application.
*/
func (a *Application) Wait() error {
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, os.Interrupt, os.Kill, syscall.SIGHUP, syscall.SIGTERM)

	for sig := range signalChan {
		if sig == syscall.SIGHUP {
			newConfig, err := config.Get()
			if err != nil {
				log.Printf("Gettin new config error: %s", err)
				continue
			}
			err = a.Reload(newConfig)
			if err != nil {
				log.Printf("Reloading failed: %s", err)
			}
		} else {
			log.Printf("Stopping %d: %s", os.Getpid(), sig)
			break
		}
	}

	if err := a.Stop(); err != nil {
		return err
	}

	return nil
}

func New(cfg *config.Config) (*Application, error) {
	return &Application{cfg: cfg}, nil
}
