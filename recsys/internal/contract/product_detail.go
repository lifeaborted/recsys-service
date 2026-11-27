package contract

import (
	"context"
	"errors"
)

// ErrProductNotFound is returned when neither feature nor offer row matches.
var ErrProductNotFound = errors.New("product not found")

type Parameter struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// ProductDetail is the full stand catalog card for one recommended item.
type ProductDetail struct {
	ItemID       string      `json:"item_id"`
	OfferID      string      `json:"offer_id"`
	SKU          string      `json:"sku"`
	Title        string      `json:"title"`
	Category     string      `json:"category"`
	Vendor       string      `json:"vendor"`
	Price        *float64    `json:"price,omitempty"`
	OldPrice     *float64    `json:"old_price,omitempty"`
	CurrencyCode string      `json:"currency_code"`
	Available    bool        `json:"available"`
	URL          string      `json:"url"`
	Description  string      `json:"description"`
	PictureURL   string      `json:"picture_url"`
	VendorURL    string      `json:"vendor_url"`
	Delivery     bool        `json:"delivery"`
	Pickup       bool        `json:"pickup"`
	SalesNotes   string      `json:"sales_notes"`
	Parameters   []Parameter `json:"parameters"`
}

// ProductDetailStore reads the full catalog card by item_id or offer_id.
type ProductDetailStore interface {
	ProductDetail(ctx context.Context, itemID string) (ProductDetail, error)
}
