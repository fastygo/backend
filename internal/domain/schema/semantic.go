package schema

import codex "github.com/fastygo/codex/schema"

// Semantic is the effective scalar meaning of a Codex field: its base type
// refined by fastygo.codex/* rules and the media UI hint. It is derived, never
// declared; manifests only use Codex types and rules.
type Semantic string

const (
	SemanticString     Semantic = "string"
	SemanticText       Semantic = "text"
	SemanticRichText   Semantic = "richtext"
	SemanticMarkdown   Semantic = "markdown"
	SemanticBoolean    Semantic = "boolean"
	SemanticNumber     Semantic = "number"
	SemanticInteger    Semantic = "integer"
	SemanticDecimal    Semantic = "decimal"
	SemanticMoney      Semantic = "money"
	SemanticDate       Semantic = "date"
	SemanticDateTime   Semantic = "datetime"
	SemanticURI        Semantic = "uri"
	SemanticUUID       Semantic = "uuid"
	SemanticEnum       Semantic = "enum"
	SemanticJSON       Semantic = "json"
	SemanticObject     Semantic = "object"
	SemanticCollection Semantic = "collection"
	SemanticRelation   Semantic = "relation"
	SemanticMedia      Semantic = "media"
	SemanticComputed   Semantic = "computed"
	SemanticEncrypted  Semantic = "encrypted"
)

// SemanticOf derives the effective meaning of a Codex field.
func SemanticOf(field Field) Semantic {
	has := func(name string) bool {
		_, ok := codex.FieldRule(field, name)
		return ok
	}
	switch field.Type {
	case FieldNumber:
		switch {
		case has(codex.RuleMoney):
			return SemanticMoney
		case has(codex.RuleInteger):
			return SemanticInteger
		case has(codex.RuleDecimal):
			return SemanticDecimal
		default:
			return SemanticNumber
		}
	case FieldDateTime:
		if has(codex.RuleDate) {
			return SemanticDate
		}
		return SemanticDateTime
	case FieldString:
		switch {
		case has(codex.RuleURI):
			return SemanticURI
		case has(codex.RuleUUID):
			return SemanticUUID
		default:
			return SemanticString
		}
	case FieldText, FieldTextarea:
		return SemanticText
	case FieldRichText:
		return SemanticRichText
	case FieldMarkdown:
		return SemanticMarkdown
	case FieldBoolean:
		return SemanticBoolean
	case FieldSelect:
		return SemanticEnum
	case FieldJSON:
		return SemanticJSON
	case FieldObject:
		return SemanticObject
	case FieldCollection:
		return SemanticCollection
	case FieldRelation:
		if IsMedia(field) {
			return SemanticMedia
		}
		return SemanticRelation
	case FieldComputed:
		return SemanticComputed
	case FieldEncrypted:
		return SemanticEncrypted
	default:
		return SemanticString
	}
}

// IsMedia reports a Codex media field (relation + ui_hint media).
func IsMedia(field Field) bool {
	return field.Type == FieldRelation && field.UIHint == codex.UIHintMedia
}

// Nullable reports the fastygo.codex/nullable rule.
func Nullable(field Field) bool {
	_, ok := codex.FieldRule(field, codex.RuleNullable)
	return ok
}

// ReadOnly reports the fastygo.codex/read-only rule.
func ReadOnly(field Field) bool {
	_, ok := codex.FieldRule(field, codex.RuleReadOnly)
	return ok
}

// EnumValues returns select option values in declaration order.
func EnumValues(field Field) []string {
	values := make([]string, 0, len(field.Options))
	for _, option := range field.Options {
		values = append(values, option.Value)
	}
	return values
}

// Many reports whether a relation stores an array of ids.
func Many(relation Relation) bool {
	return relation.Cardinality != OneToOne
}

// OnDelete returns the relation delete behavior (restrict when unset).
func OnDelete(relation Relation) DeleteBehavior {
	if relation.DeleteBehavior == "" {
		return DeleteRestrict
	}
	return relation.DeleteBehavior
}

// Options builds select options whose labels equal their values.
func Options(values ...string) []Option {
	options := make([]Option, 0, len(values))
	for _, value := range values {
		options = append(options, Option{Value: value, Label: value})
	}
	return options
}

// Rules builds rule declarations from Codex rule names.
func Rules(names ...string) []ValidationRule {
	rules := make([]ValidationRule, 0, len(names))
	for _, name := range names {
		rules = append(rules, ValidationRule{Name: name})
	}
	return rules
}

// Codex rule names, re-exported for manifests built in Go.
const (
	RuleInteger  = codex.RuleInteger
	RuleDecimal  = codex.RuleDecimal
	RuleMoney    = codex.RuleMoney
	RuleDate     = codex.RuleDate
	RuleURI      = codex.RuleURI
	RuleUUID     = codex.RuleUUID
	RuleNullable = codex.RuleNullable
	RuleReadOnly = codex.RuleReadOnly
	RuleJSONAny  = codex.RuleJSONAny
	UIHintMedia  = codex.UIHintMedia
)
