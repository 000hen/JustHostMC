package grpcsvc

import (
	"fmt"
	"strconv"

	mcmanagerv1 "github.com/000hen/justhostmc/engine/gen/mcmanager/v1"
	"github.com/000hen/justhostmc/engine/internal/scripting"
	"google.golang.org/grpc/codes"
)

// configOptions maps a script's declared typed config options to their proto
// form (nil when the script declares none).
func configOptions(opts []scripting.ConfigOption) []*mcmanagerv1.ConfigOption {
	if len(opts) == 0 {
		return nil
	}
	out := make([]*mcmanagerv1.ConfigOption, 0, len(opts))
	for _, o := range opts {
		out = append(out, &mcmanagerv1.ConfigOption{
			Key:         o.Key,
			Type:        o.Type,
			Name:        o.Name,
			Description: o.Description,
			Default:     o.Default,
			Required:    o.Required,
		})
	}
	return out
}

// getConfigView builds the ScriptConfig get-reply for id: one value per declared
// option. Secrets are reported only as present (has_value), never their value.
func getConfigView(id string, opts []scripting.ConfigOption, store *scripting.ConfigStore) *mcmanagerv1.ScriptConfig {
	var stored map[string]string
	if store != nil {
		stored = store.Values(id)
	}
	values := make([]*mcmanagerv1.ScriptConfigValue, 0, len(opts))
	for _, o := range opts {
		v, has := stored[o.Key]
		out := &mcmanagerv1.ScriptConfigValue{Key: o.Key, HasValue: has}
		if o.Type != mcmanagerv1.ConfigOptionType_CONFIG_OPTION_SECRET {
			out.Value = v
		}
		values = append(values, out)
	}
	return &mcmanagerv1.ScriptConfig{Id: id, Values: values}
}

// applyConfig validates and persists the requested overrides for id, then
// returns the refreshed get-view. Unknown keys and values that don't parse for
// their declared type are rejected; an empty value clears the override.
func applyConfig(id string, opts []scripting.ConfigOption, store *scripting.ConfigStore, req *mcmanagerv1.SetConfigRequest) (*mcmanagerv1.ScriptConfig, error) {
	if store == nil {
		return nil, errorStatus(codes.FailedPrecondition, mcmanagerv1.ErrorCode_ERROR_CODE_UNSPECIFIED, "config store not available", nil)
	}
	byKey := make(map[string]scripting.ConfigOption, len(opts))
	for _, o := range opts {
		byKey[o.Key] = o
	}
	for _, v := range req.Values {
		o, ok := byKey[v.Key]
		if !ok {
			return nil, errorStatus(codes.InvalidArgument, mcmanagerv1.ErrorCode_ERROR_CODE_UNSPECIFIED, fmt.Sprintf("unknown config key %q", v.Key), nil)
		}
		if v.Value != "" {
			if err := validateConfigValue(o.Type, v.Value); err != nil {
				return nil, errorStatus(codes.InvalidArgument, mcmanagerv1.ErrorCode_ERROR_CODE_UNSPECIFIED, err.Error(), nil)
			}
		}
		if err := store.Set(id, v.Key, v.Value); err != nil {
			return nil, errorStatus(codes.Internal, mcmanagerv1.ErrorCode_ERROR_CODE_UNSPECIFIED, err.Error(), nil)
		}
	}
	return getConfigView(id, opts, store), nil
}

// validateConfigValue enforces that number/boolean overrides parse.
func validateConfigValue(t mcmanagerv1.ConfigOptionType, val string) error {
	switch t {
	case mcmanagerv1.ConfigOptionType_CONFIG_OPTION_NUMBER:
		if _, err := strconv.ParseFloat(val, 64); err != nil {
			return fmt.Errorf("value %q is not a number", val)
		}
	case mcmanagerv1.ConfigOptionType_CONFIG_OPTION_BOOLEAN:
		if _, err := strconv.ParseBool(val); err != nil {
			return fmt.Errorf("value %q is not a boolean", val)
		}
	}
	return nil
}
