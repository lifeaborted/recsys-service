package enrichment

import (
	"bufio"
	"compress/gzip"
	"encoding/xml"
	"fmt"
	"io"
	"net/url"
	"path"
	"strings"
	"time"
)

type FeedVersion struct {
	ID          string    `json:"id"`
	URL         string    `json:"url"`
	GeneratedAt time.Time `json:"generated_at"`
}

type Source struct {
	URL        string    `json:"url"`
	Quote      string    `json:"quote"`
	Version    string    `json:"version"`
	ObservedAt time.Time `json:"observed_at"`
	Locator    string    `json:"locator,omitempty"`
}

type Fact struct {
	Value  *string `json:"value"`
	Unit   string  `json:"unit,omitempty"`
	Source *Source `json:"source"`
}

type Offer struct {
	ID    string          `json:"offer_id"`
	SKU   string          `json:"sku"`
	Name  string          `json:"name"`
	URL   string          `json:"url"`
	Facts map[string]Fact `json:"facts"`
}

type feedOffer struct {
	ID         string `xml:"id,attr"`
	Available  string `xml:"available,attr"`
	SKU        string `xml:"vendorCode"`
	Name       string `xml:"name"`
	URL        string `xml:"url"`
	Category   string `xml:"categoryId"`
	Vendor     string `xml:"vendor"`
	Parameters []struct {
		Name  string `xml:"name,attr"`
		Unit  string `xml:"unit,attr"`
		Value string `xml:",chardata"`
	} `xml:"param"`
}

var parameterNames = map[string]string{
	"family": "family", "семейство": "family",
	"edition": "edition", "редакция": "edition",
	"platform": "platform", "платформа": "platform",
	"license type": "license_type", "тип лицензии": "license_type",
	"seats": "seats", "места": "seats",
	"term": "term", "срок": "term",
}

// ReadFeed visits offers in source order. Publish the resulting batch only after it returns nil.
func ReadFeed(r io.Reader, version, sourceURL string, location *time.Location, visit func(FeedVersion, Offer) error) (FeedVersion, error) {
	var feed FeedVersion
	if strings.TrimSpace(version) == "" || !webURL(sourceURL) || location == nil || visit == nil {
		return feed, fmt.Errorf("feed requires a version, source URL, timezone and visitor")
	}
	input := bufio.NewReader(r)
	var data io.Reader = input
	if prefix, _ := input.Peek(2); len(prefix) == 2 && prefix[0] == 0x1f && prefix[1] == 0x8b {
		compressed, err := gzip.NewReader(input)
		if err != nil {
			return feed, err
		}
		defer compressed.Close()
		data = compressed
	}
	decoder := xml.NewDecoder(data)
	var stack []string
	seen := make(map[string]struct{})
	closed := false
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			if !closed || len(stack) != 0 {
				return feed, fmt.Errorf("incomplete YML catalog")
			}
			return feed, nil
		}
		if err != nil {
			return feed, err
		}
		switch node := token.(type) {
		case xml.StartElement:
			if len(stack) == 0 {
				if closed || node.Name.Local != "yml_catalog" {
					return feed, fmt.Errorf("expected yml_catalog root")
				}
				var date string
				for _, attr := range node.Attr {
					if attr.Name.Local == "date" {
						date = attr.Value
					}
				}
				generated, err := time.Parse(time.RFC3339, date)
				if err != nil {
					generated, err = time.ParseInLocation("2006-01-02 15:04", date, location)
				}
				if err != nil {
					return feed, fmt.Errorf("invalid yml_catalog date: %w", err)
				}
				feed = FeedVersion{ID: version, URL: sourceURL, GeneratedAt: generated}
			}
			if len(stack) == 3 && stack[1] == "shop" && stack[2] == "offers" && node.Name.Local == "offer" {
				var raw feedOffer
				if err := decoder.DecodeElement(&raw, &node); err != nil {
					return feed, err
				}
				raw.ID = strings.TrimSpace(raw.ID)
				if raw.ID == "" {
					return feed, fmt.Errorf("offer without id")
				}
				if _, duplicate := seen[raw.ID]; duplicate {
					return feed, fmt.Errorf("duplicate offer id %q", raw.ID)
				}
				seen[raw.ID] = struct{}{}
				offer, err := raw.normalize(feed)
				if err != nil {
					return feed, err
				}
				if err := visit(feed, offer); err != nil {
					return feed, err
				}
				continue
			}
			stack = append(stack, node.Name.Local)
		case xml.EndElement:
			stack = stack[:len(stack)-1]
			closed = len(stack) == 0
		}
	}
}

func (raw feedOffer) normalize(feed FeedVersion) (Offer, error) {
	values := map[string]string{
		"category": raw.Category, "available": raw.Available, "vendor": raw.Vendor,
		"family": "", "edition": "", "platform": "", "license_type": "", "seats": "", "term": "",
	}
	units := make(map[string]string)
	available := strings.TrimSpace(raw.Available)
	if available != "" && available != "true" && available != "false" {
		return Offer{}, fmt.Errorf("offer %q: invalid available", raw.ID)
	}
	for _, parameter := range raw.Parameters {
		field := parameterNames[strings.ToLower(strings.TrimSpace(parameter.Name))]
		value := strings.TrimSpace(parameter.Value)
		if field == "" || value == "" {
			continue
		}
		unit := strings.ToLower(strings.TrimSpace(parameter.Unit))
		if values[field] != "" && (values[field] != value || units[field] != unit) {
			return Offer{}, fmt.Errorf("offer %q: conflicting %s", raw.ID, field)
		}
		values[field], units[field] = value, unit
	}
	offer := Offer{ID: raw.ID, SKU: strings.TrimSpace(raw.SKU), Name: strings.TrimSpace(raw.Name),
		URL: strings.TrimSpace(raw.URL), Facts: make(map[string]Fact)}
	for field, rawValue := range values {
		value := strings.TrimSpace(rawValue)
		fact := Fact{}
		if value != "" {
			fact = Fact{Value: &value, Unit: units[field], Source: &Source{URL: feed.URL, Quote: value, Version: feed.ID,
				ObservedAt: feed.GeneratedAt, Locator: "offer/" + offer.ID + "/" + field}}
		}
		offer.Facts[field] = fact
	}
	return offer, nil
}

func webURL(s string) bool {
	u, err := url.Parse(s)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Hostname() != "" && u.User == nil
}

func urlSKU(s string) string {
	if !webURL(s) {
		return ""
	}
	u, _ := url.Parse(s)
	slug := path.Base(strings.TrimRight(u.Path, "/"))
	i := strings.LastIndexByte(slug, '-')
	if i < 1 || i == len(slug)-1 {
		return ""
	}
	for _, ch := range slug[i+1:] {
		if ch < '0' || ch > '9' {
			return ""
		}
	}
	return normalizeSKU(slug[:i])
}
