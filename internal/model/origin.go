package model

type Origin string

const (
	OriginRegistry Origin = "registry"
	OriginAccounts Origin = "accounts"
	OriginRoles    Origin = "roles"
	OriginPage     Origin = "page"
	OriginLLMClaim Origin = "llm_claim"
)

func (o Origin) IsValid() bool {
	switch o {
	case OriginRegistry, OriginAccounts, OriginRoles, OriginPage, OriginLLMClaim:
		return true
	default:
		return false
	}
}
