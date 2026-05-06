package contextasm

import (
	"fmt"
	"strings"
)

func AssemblePackage(input PackageInput) (PromptPackage, error) {
	scope := input.Scope.normalized()
	if err := scope.Validate(); err != nil {
		return PromptPackage{}, err
	}

	pkg := PromptPackage{
		Scope:          scope,
		IncludedBlocks: []IncludedBlock{},
		SourceRefs:     []SourceRef{},
		Conflicts:      []InstructionConflict{},
	}
	if len(input.Selections) == 0 && len(input.Conflicts) == 0 {
		return pkg, nil
	}

	seenRefs := make(map[string]struct{})
	for _, selection := range input.Selections {
		whySelected := strings.TrimSpace(selection.WhySelected)
		if whySelected == "" {
			return PromptPackage{}, fmt.Errorf("%w: why_selected is required", ErrInvalidInput)
		}
		block := selection.Block.normalized()
		if err := block.Validate(); err != nil {
			return PromptPackage{}, err
		}
		visible, err := IsVisibleToScope(scope, block)
		if err != nil {
			return PromptPackage{}, err
		}
		if !visible {
			return PromptPackage{}, fmt.Errorf("%w: source block %q is not visible to scope %q", ErrInvalidInput, block.ID, scope.Kind)
		}
		tokenEstimate := block.TokenEstimate
		if tokenEstimate <= 0 {
			tokenEstimate = approximateTokens(block.Title + "\n" + block.Content)
		}
		pkg.IncludedBlocks = append(pkg.IncludedBlocks, IncludedBlock{
			Block:         block,
			WhySelected:   whySelected,
			TokenEstimate: tokenEstimate,
		})
		pkg.TotalTokenEstimate += tokenEstimate
		for _, ref := range block.SourceRefs {
			if _, exists := seenRefs[ref.Ref]; exists {
				continue
			}
			seenRefs[ref.Ref] = struct{}{}
			pkg.SourceRefs = append(pkg.SourceRefs, ref)
		}
	}

	for _, conflict := range input.Conflicts {
		conflict = conflict.normalized()
		if err := conflict.Validate(); err != nil {
			return PromptPackage{}, err
		}
		pkg.Conflicts = append(pkg.Conflicts, conflict)
	}

	return pkg, nil
}

func approximateTokens(text string) int {
	text = strings.TrimSpace(text)
	if text == "" {
		return 0
	}
	runes := []rune(text)
	estimate := (len(runes) + 3) / 4
	if estimate == 0 {
		return 1
	}
	return estimate
}
