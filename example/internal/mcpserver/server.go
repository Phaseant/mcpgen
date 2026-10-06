package mcpserver

import (
	"context"
	"slices"
	"sort"
	"strings"

	"github.com/phaseant/mcpgen/example/internal/mcpapi"
	"github.com/phaseant/mcpgen/example/internal/service"
)

type Handler struct{ pets *service.Pets }

func New(pets *service.Pets) *Handler { return &Handler{pets: pets} }

var _ mcpapi.Handler = (*Handler)(nil)

func (*Handler) Echo(_ context.Context, input mcpapi.EchoInput) (mcpapi.EchoOutput, error) {
	return mcpapi.EchoOutput{Message: input.Message}, nil
}

func (h *Handler) GetPet(_ context.Context, input mcpapi.GetPetInput) (mcpapi.Pet, error) {
	pet, ok := h.pets.Get(input.PetID)
	if !ok {
		return mcpapi.Pet{}, mcpapi.NewToolError("pet_not_found", "pet does not exist")
	}
	return mcpapi.Pet{ID: pet.ID, Name: pet.Name, Kind: mcpapi.PetKind(pet.Kind)}, nil
}

func (h *Handler) CreatePet(_ context.Context, input mcpapi.CreatePetRequest) (mcpapi.Pet, error) {
	pet := h.pets.Create(input.Name, string(input.Kind))
	return mcpapi.Pet{ID: pet.ID, Name: pet.Name, Kind: mcpapi.PetKind(pet.Kind)}, nil
}

func (h *Handler) Search(_ context.Context, input mcpapi.SearchInput) (mcpapi.SearchResult, error) {
	pets := h.pets.List()
	if input.Sort == nil || *input.Sort == mcpapi.SearchInputSortRelevance {
		sort.SliceStable(pets, func(i, j int) bool { return pets[i].Name < pets[j].Name })
	}
	limit := 100
	if input.Limit != nil {
		limit = int(*input.Limit)
	}
	output := mcpapi.SearchResult{Pets: make([]mcpapi.Pet, 0)}
	for _, pet := range pets {
		if !strings.Contains(strings.ToLower(pet.Name), strings.ToLower(input.Query)) {
			continue
		}
		if input.Filter != nil && input.Filter.IDs != nil && !slices.Contains(*input.Filter.IDs, pet.ID) {
			continue
		}
		output.Pets = append(output.Pets, mcpapi.Pet{ID: pet.ID, Name: pet.Name, Kind: mcpapi.PetKind(pet.Kind)})
		if len(output.Pets) == limit {
			break
		}
	}
	return output, nil
}
