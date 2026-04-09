package model

type CapabilityProfile string

const (
	CapabilityProfileNone         CapabilityProfile = "none"
	CapabilityProfileArkResponses CapabilityProfile = "ark_responses"
)

type ProviderCapabilities struct {
	Profile              CapabilityProfile
	SupportsSessionCache bool
	SupportsPrefixCache  bool
	CachesToolResults    bool
	RotatesSessionRef    bool
}

type CacheMode string

const (
	CacheModeSession CacheMode = "session"
	CacheModePrefix  CacheMode = "prefix"
)

type CacheDirective struct {
	Mode               CacheMode
	Store              bool
	PreviousResponseID string
	ExpireAt           int64
}

type ResponseMetadata struct {
	ResponseID   string
	CachedTokens int
	InputTokens  int
	OutputTokens int
}
