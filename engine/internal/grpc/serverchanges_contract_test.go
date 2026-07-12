package grpcsvc

import (
	"testing"

	mb "github.com/000hen/justhostmc/engine/gen/mcmanager/v1"
)

func TestServerChangeContractHasSingleItemVariants(t *testing.T) {
	tests := []struct {
		name  string
		event *mb.ServerChangeEvent
	}{
		{"ready", &mb.ServerChangeEvent{Change: &mb.ServerChangeEvent_Ready{Ready: &mb.Empty{}}}},
		{"upsert", &mb.ServerChangeEvent{Change: &mb.ServerChangeEvent_Upsert{Upsert: &mb.Server{Id: "one"}}}},
		{"deleted", &mb.ServerChangeEvent{Change: &mb.ServerChangeEvent_Deleted{Deleted: &mb.ServerId{Id: "one"}}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.event.GetChange() == nil {
				t.Fatal("change variant is nil")
			}
		})
	}
}
