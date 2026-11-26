// Package domain defines the data exchanged by the recommendation components.
package domain

import "time"

type ItemID string
type ClientID string
type ClientGroup string

const (
	B2B ClientGroup = "B2B"
	B2C ClientGroup = "B2C"
)

type Client struct {
	ID          ClientID
	Group       ClientGroup
	Type        string
	Industry    *string
	Segment     *string
	HasLKAccess bool
}

type Sale struct {
	SourceRow int
	OrderID   string
	ClientID  ClientID
	OrderWeek time.Time
	ItemID    ItemID
	SKU       *string
	ItemName  string
}
