package types

import "extension-scaffold/pkg/decoder"

// RegisterDecoders registers all type decoders for this extension.
func RegisterDecoders(r *decoder.Registry) {
	// CLASSIFY_SEVERITY message (ABI-encoded)
	r.Register(
		decoder.RegistryKey{OPType: "INCIDENT_TRIAGE", OPCommand: "CLASSIFY_SEVERITY", Kind: decoder.KindMessage},
		decoder.NewABIDecoder[ClassifySeverityRequest](ClassifySeverityMessageArg),
	)
	// CLASSIFY_SEVERITY result (JSON)
	r.Register(
		decoder.RegistryKey{OPType: "INCIDENT_TRIAGE", OPCommand: "CLASSIFY_SEVERITY", Kind: decoder.KindResult},
		decoder.NewJSONDecoder[ClassifySeverityResponse](),
	)
}
