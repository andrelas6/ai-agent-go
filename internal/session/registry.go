package session

import (
	"sync"

	"github.com/google/uuid"
)

type Registry struct {
	mu       sync.Mutex
	sessions map[string]chan string
}

func NewRegistry() *Registry {
	return &Registry{sessions: make(map[string]chan string)}
}

func (r *Registry) Create() (id string, ch chan string) {
	id = uuid.NewString()
	ch = make(chan string, 1)
	r.mu.Lock()
	r.sessions[id] = ch
	r.mu.Unlock()
	return id, ch
}

func (r *Registry) Get(id string) (chan string, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	ch, ok := r.sessions[id]
	return ch, ok
}

func (r *Registry) Delete(id string) {
	r.mu.Lock()
	delete(r.sessions, id)
	r.mu.Unlock()
}
