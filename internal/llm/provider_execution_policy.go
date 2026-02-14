package llm

import "github.com/danshapiro/kilroy/internal/providerspec"

const kimiCodingMinMaxTokens = 16000

type ProviderExecutionPolicy struct {
	ForceStream  bool
	MinMaxTokens int
	Reason       string
}

func ExecutionPolicy(provider string) ProviderExecutionPolicy {
	switch providerspec.CanonicalProviderKey(provider) {
	case "kimi":
		return ProviderExecutionPolicy{
			ForceStream:  true,
			MinMaxTokens: kimiCodingMinMaxTokens,
			Reason: "Kimi Coding requests must use stream=true with max_tokens>=16000 " +
				"to avoid tool-history continuation failures that often surface as misleading 429 overload errors.",
		}
	default:
		return ProviderExecutionPolicy{}
	}
}

func ApplyExecutionPolicy(req Request, policy ProviderExecutionPolicy) Request {
	if policy.MinMaxTokens <= 0 {
		return req
	}
	current := 0
	if req.MaxTokens != nil {
		current = *req.MaxTokens
	}
	if current >= policy.MinMaxTokens {
		return req
	}
	v := policy.MinMaxTokens
	req.MaxTokens = &v
	return req
}
