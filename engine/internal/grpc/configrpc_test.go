package grpcsvc

import (
	"path/filepath"
	"testing"

	mcmanagerv1 "github.com/000hen/justhostmc/engine/gen/mcmanager/v1"
	"github.com/000hen/justhostmc/engine/internal/scripting"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func cfgOpts() []scripting.ConfigOption {
	return []scripting.ConfigOption{
		{Key: "region", Type: mcmanagerv1.ConfigOptionType_CONFIG_OPTION_STRING, Default: "us"},
		{Key: "count", Type: mcmanagerv1.ConfigOptionType_CONFIG_OPTION_NUMBER},
		{Key: "token", Type: mcmanagerv1.ConfigOptionType_CONFIG_OPTION_SECRET},
	}
}

func valueByKey(sc *mcmanagerv1.ScriptConfig, key string) *mcmanagerv1.ScriptConfigValue {
	for _, v := range sc.Values {
		if v.Key == key {
			return v
		}
	}
	return nil
}

func setReq(id string, kv ...string) *mcmanagerv1.SetConfigRequest {
	req := &mcmanagerv1.SetConfigRequest{Id: id}
	for i := 0; i+1 < len(kv); i += 2 {
		req.Values = append(req.Values, &mcmanagerv1.ScriptConfigValue{Key: kv[i], Value: kv[i+1]})
	}
	return req
}

func TestConfigRPCRoundTrip(t *testing.T) {
	opts := cfgOpts()
	store := scripting.NewConfigStore(filepath.Join(t.TempDir(), "c.json"))

	// Nothing stored: every declared option appears, none has a value.
	view := getConfigView("id", opts, store)
	if len(view.Values) != 3 {
		t.Fatalf("values = %d, want 3", len(view.Values))
	}
	if v := valueByKey(view, "region"); v.HasValue || v.Value != "" {
		t.Fatalf("unset region: %+v", v)
	}

	// Set a string and a secret.
	view, err := applyConfig("id", opts, store, setReq("id", "region", "eu", "token", "secret123"))
	if err != nil {
		t.Fatal(err)
	}
	if v := valueByKey(view, "region"); !v.HasValue || v.Value != "eu" {
		t.Fatalf("region after set: %+v", v)
	}
	// Secret is present but never echoed back.
	if v := valueByKey(view, "token"); !v.HasValue || v.Value != "" {
		t.Fatalf("secret must report present without value: %+v", v)
	}

	// Unknown key is rejected.
	if _, err := applyConfig("id", opts, store, setReq("id", "nope", "x")); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("unknown key err = %v, want InvalidArgument", err)
	}
	// A non-numeric value for a number option is rejected.
	if _, err := applyConfig("id", opts, store, setReq("id", "count", "abc")); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("bad number err = %v, want InvalidArgument", err)
	}

	// An empty value clears the override.
	view, err = applyConfig("id", opts, store, setReq("id", "region", ""))
	if err != nil {
		t.Fatal(err)
	}
	if v := valueByKey(view, "region"); v.HasValue {
		t.Fatalf("region should be cleared: %+v", v)
	}

	// A nil store cannot be written.
	if _, err := applyConfig("id", opts, nil, setReq("id", "region", "eu")); status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("nil store err = %v, want FailedPrecondition", err)
	}
}
