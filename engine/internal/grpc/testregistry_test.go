package grpcsvc

import (
	"github.com/000hen/justhostmc/engine/internal/provider"
	"github.com/000hen/justhostmc/engine/internal/scripting"
)

// testRegistry builds a provider registry holding a single stub provider under
// id (with the given mod layout) — the test replacement for the old
// map[ServerType]provider.Provider injection.
func testRegistry(id, modLayout string, prov provider.Provider) *scripting.Registry {
	r := scripting.NewRegistry(scripting.NewHost(nil, nil, nil), nil)
	r.AddEntry(&scripting.Entry{
		Meta:     scripting.Meta{ID: id, ModLayout: modLayout},
		Provider: prov,
		Builtin:  true,
	})
	return r
}
