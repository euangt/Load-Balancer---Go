package backend

import (
    "net/url"
    "net/http/httputil"
    "sync"
)

type Backend struct {
  URL          *url.URL
  Alive        bool
  mux          sync.RWMutex
  ReverseProxy *httputil.ReverseProxy
}

func (backend *Backend) SetAlive(alive bool) {
    backend.mux.Lock()
	backend.Alive = alive
	backend.mux.Unlock()
}

func (backend *Backend) IsAlive() bool {
    backend.mux.RLock()
    alive := backend.Alive
    backend.mux.RUnlock()

    return alive
}
