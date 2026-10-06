package service

import (
	"strconv"
	"sync"
)

type Pet struct{ ID, Name, Kind string }

// Pets is the example application's in-memory store, ordered by creation.
type Pets struct {
	mu   sync.RWMutex
	pets []Pet
}

func (s *Pets) Create(name, kind string) Pet {
	s.mu.Lock()
	defer s.mu.Unlock()
	pet := Pet{ID: strconv.Itoa(len(s.pets) + 1), Name: name, Kind: kind}
	s.pets = append(s.pets, pet)
	return pet
}

func (s *Pets) Get(id string) (Pet, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, pet := range s.pets {
		if pet.ID == id {
			return pet, true
		}
	}
	return Pet{}, false
}

func (s *Pets) List() []Pet {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]Pet(nil), s.pets...)
}
