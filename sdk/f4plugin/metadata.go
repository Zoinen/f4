package f4plugin

// MetadataFields records which optional values a provider actually obtained.
// MetadataExplicit makes missing bits authoritative, including for synthetic
// fallback timestamps. Without it, old providers retain conservative inference.
type MetadataFields = uint32

const (
	MetadataPhysicalSize MetadataFields = 1 << iota
	MetadataPermissions
	MetadataUID
	MetadataGID
	MetadataWinAttrs
	MetadataHidden
	MetadataExecutable
	MetadataMTime
	MetadataATime
	MetadataCTime
	MetadataExplicit MetadataFields = 1 << 31
)
