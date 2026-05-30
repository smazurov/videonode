package events

import (
	"reflect"
	"strings"

	"github.com/danielgtaylor/huma/v2"
)

// refPrefix is huma's default component-schema ref prefix. Arm refs and the
// discriminator mapping must use it; openapi-typescript requires the mapping
// to carry these refs (otherwise it emits schema names, not wire values).
const refPrefix = "#/components/schemas/"

// entityVariant is one arm of the EntityEvent discriminated union: a wire tag
// ("<entity>.<action>") and the Go type carried in Payload (nil for deletes).
type entityVariant struct {
	entityType string
	action     string
	tag        string
	payload    reflect.Type
}

var entityVariants []entityVariant

// RegisterVariant declares an EntityEvent arm carrying a P payload. Call
// before the OpenAPI schema is built (see internal/api). Storing only a
// reflect.Type keeps this package free of any payload-package import.
func RegisterVariant[P any](entityType, action string) {
	entityVariants = append(entityVariants, entityVariant{
		entityType: entityType,
		action:     action,
		tag:        entityType + "." + action,
		payload:    reflect.TypeFor[P](),
	})
}

// RegisterDeleteVariant declares a payload-less "<entity>.deleted" arm.
func RegisterDeleteVariant(entityType string) {
	entityVariants = append(entityVariants, entityVariant{
		entityType: entityType,
		action:     "deleted",
		tag:        entityType + ".deleted",
	})
}

// Schema makes EntityEvent a huma.SchemaProvider: instead of reflecting the
// envelope into a free-form `payload: any`, it emits a oneOf+discriminator
// over the registered variants, which openapi-typescript renders as a TS
// discriminated union narrowing on `type`.
func (EntityEvent) Schema(r huma.Registry) *huma.Schema {
	arms := make([]*huma.Schema, 0, len(entityVariants))
	mapping := make(map[string]string, len(entityVariants))
	for _, v := range entityVariants {
		props := map[string]*huma.Schema{
			"type":      {Type: huma.TypeString, Enum: []any{v.tag}},
			"id":        {Type: huma.TypeString},
			"timestamp": {Type: huma.TypeString},
		}
		required := []string{"type", "id", "timestamp"}
		if v.payload != nil {
			props["payload"] = r.Schema(v.payload, true, v.payload.Name())
			required = append(required, "payload")
		}
		armName := "Entity" + title(v.entityType) + title(v.action)
		armRef := refPrefix + armName
		// Idempotent: Schema may be invoked more than once during generation.
		if _, ok := r.Map()[armName]; !ok {
			r.Map()[armName] = &huma.Schema{
				Type:       huma.TypeObject,
				Properties: props,
				Required:   required,
			}
		}
		arms = append(arms, &huma.Schema{Ref: armRef})
		mapping[v.tag] = armRef
	}
	return &huma.Schema{
		OneOf:         arms,
		Discriminator: &huma.Discriminator{PropertyName: "type", Mapping: mapping},
	}
}

func title(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}
