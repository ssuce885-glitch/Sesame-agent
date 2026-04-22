package setupflow

import "fmt"

func providerForVendor(vendor, compat string) (string, error) {
	switch vendor {
	case "anthropic", "minimax":
		return "anthropic", nil
	case "openai", "volcengine":
		return "openai_compatible", nil
	case "fake":
		return "fake", nil
	case "custom":
		switch compat {
		case "anthropic":
			return "anthropic", nil
		case "openai":
			return "openai_compatible", nil
		default:
			return "", fmt.Errorf("unknown compatibility %q", compat)
		}
	default:
		return "", fmt.Errorf("unknown vendor %q", vendor)
	}
}
