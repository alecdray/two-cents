package views

import (
	"hash/fnv"

	"github.com/alecdray/two-cents/src/internal/categorization"
)

// avatarVisual is the glyph + color a transaction row's leading avatar uses when
// no merchant logo is available. Glyph is a Bootstrap Icons name; Color is a
// categorical-palette text utility (text-category-*) that tints the glyph.
type avatarVisual struct {
	Glyph string
	Color string
}

// builtinCategoryVisuals maps each built-in spending Category onto a distinct
// glyph and a distinct categorical hue, so the twelve built-ins are told apart at
// a glance. Keyed by the stable Category ids exported from the categorization
// package; keep it in lockstep with that set.
var builtinCategoryVisuals = map[string]avatarVisual{
	categorization.CategoryFoodAndDrink:           {Glyph: "cup-straw", Color: "text-category-1"},
	categorization.CategoryGeneralMerchandise:     {Glyph: "bag", Color: "text-category-2"},
	categorization.CategoryTransportation:         {Glyph: "car-front", Color: "text-category-3"},
	categorization.CategoryTravel:                 {Glyph: "airplane", Color: "text-category-4"},
	categorization.CategoryRentAndUtilities:       {Glyph: "house-door", Color: "text-category-5"},
	categorization.CategoryMedical:                {Glyph: "heart-pulse", Color: "text-category-6"},
	categorization.CategoryPersonalCare:           {Glyph: "droplet", Color: "text-category-7"},
	categorization.CategoryGeneralServices:        {Glyph: "tools", Color: "text-category-8"},
	categorization.CategoryEntertainment:          {Glyph: "film", Color: "text-category-9"},
	categorization.CategoryHomeImprovement:        {Glyph: "hammer", Color: "text-category-10"},
	categorization.CategoryBankFees:               {Glyph: "percent", Color: "text-category-11"},
	categorization.CategoryGovernmentAndNonProfit: {Glyph: "bank", Color: "text-category-12"},
}

// customCategoryPalette is the set of hues a custom (user-created) Category hashes
// into. A custom Category has no fixed hue, so its color is chosen deterministically
// from its id — stable across renders, though two custom Categories may collide.
var customCategoryPalette = []string{
	"text-category-1", "text-category-2", "text-category-3", "text-category-4",
	"text-category-5", "text-category-6", "text-category-7", "text-category-8",
	"text-category-9", "text-category-10", "text-category-11", "text-category-12",
}

// customCategoryGlyph is the generic glyph a custom Category shows — it has no
// built-in identity, so it reads simply as a labeled bucket.
const customCategoryGlyph = "tag"

// defaultCategoryVisual is the avatar for a row with no category — needs-review,
// or an as-yet uncategorized spend. One fixed glyph and a neutral hue, so no row
// is ever blank and the no-category state reads uniformly.
var defaultCategoryVisual = avatarVisual{Glyph: "receipt", Color: "text-category-neutral"}

// incomeVisual is the avatar for an Income row: money coming in.
var incomeVisual = avatarVisual{Glyph: "cash-stack", Color: "text-category-income"}

// transferVisual is the avatar for a plain Transfer row: money moving between accounts.
var transferVisual = avatarVisual{Glyph: "arrow-left-right", Color: "text-category-transfer"}

// savingsTransferVisual is the avatar for a savings-contribution Transfer row.
var savingsTransferVisual = avatarVisual{Glyph: "piggy-bank", Color: "text-category-savings"}

// categoryVisual picks the avatar's glyph + color for a row. A Spending row with a
// Category shows that Category's visual — the built-in's fixed glyph/hue, or a
// custom Category's generic glyph with an id-stable hue. Income rows show the income
// visual; Transfer rows show a savings or plain transfer visual depending on their
// subtype. Every other row (needs-review, uncategorized spend) shows the neutral
// default. Only a Spending Category carries a category visual, so a stray id on a
// non-spending row can never leak a category glyph.
func categoryVisual(categoryID *string, classification categorization.Classification, subtype categorization.TransferSubtype) avatarVisual {
	switch classification {
	case categorization.Income:
		return incomeVisual
	case categorization.Transfer:
		if subtype == categorization.SubtypeSavingsContribution {
			return savingsTransferVisual
		}
		return transferVisual
	case categorization.Spending:
		if categoryID != nil {
			if v, ok := builtinCategoryVisuals[*categoryID]; ok {
				return v
			}
			return customCategoryVisual(*categoryID)
		}
	}
	return defaultCategoryVisual
}

// customCategoryVisual derives a custom Category's avatar: the generic glyph and a
// hue chosen by hashing its id into the palette, so the same Category is always the
// same color.
func customCategoryVisual(categoryID string) avatarVisual {
	h := fnv.New32a()
	_, _ = h.Write([]byte(categoryID))
	color := customCategoryPalette[h.Sum32()%uint32(len(customCategoryPalette))]
	return avatarVisual{Glyph: customCategoryGlyph, Color: color}
}
