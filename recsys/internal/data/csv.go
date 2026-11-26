package data

import (
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"
	"time"

	"recsys/internal/contract"
	"recsys/internal/domain"
)

const (
	// DefaultBatchSize limits the number of clients or sales records per DatasetBatch.
	DefaultBatchSize = 1000

	// Reference metadata for closed dataset releases v4 (current DB dump) and v5 (latest workbook).
	ExpectedV4Version        = "v4"
	ExpectedV4WorkbookSHA256 = "7e219b81f22c51a6a3060e22d7e74657034b8b1d752c3ffaa2c6fa189bb666f4"

	ExpectedV5Version        = "v5"
	ExpectedV5WorkbookSHA256 = "56e3c100cd600f1e0d8daf02989b3a0f82a633fa727d30aa49b4d778ef1b0c65"
	ExpectedV5ProductGroups  = 419

	ExpectedV4Clients    = 19008
	ExpectedV4Sales      = 31753
	ExpectedV4Items      = 817
	ExpectedV4Baskets    = 27603
	ExpectedV4TrainItems = 813
	ExpectedV4ColdItems  = 4
)

var (
	expectedClientsHeader = []string{"client_id", "client_group", "client_type", "industry", "segment", "lk_access"}
	expectedSalesHeaderV4 = []string{"order_id", "client_id", "order_week", "item_id", "sku", "item_name"}
	expectedSalesHeaderV5 = []string{"order_id", "client_id", "order_week", "item_id", "sku", "item_name", "product_group"}
)

// Dataset holds normalized clients, sales records in exact source order, and item->product_group mapping.
type Dataset struct {
	Clients       []domain.Client
	Sales         []domain.Sale
	ProductGroups map[domain.ItemID]string
	HasGroupCol   bool
}

// ImportSummary summarizes a validated and published dataset import.
type ImportSummary struct {
	Version               string
	Clients               int
	Sales                 int
	Batches               int
	DistinctItems         int
	DistinctProductGroups int
	EmptySKURows          int
	HasProductGroupColumn bool
}

// LoadCSV reads and validates clients.csv and sales.csv from disk.
func LoadCSV(ctx context.Context, clientsPath, salesPath string) (Dataset, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return Dataset{}, err
	}

	cf, err := os.Open(clientsPath)
	if err != nil {
		return Dataset{}, fmt.Errorf("open clients csv %q: %w", clientsPath, err)
	}
	defer cf.Close()

	sf, err := os.Open(salesPath)
	if err != nil {
		return Dataset{}, fmt.Errorf("open sales csv %q: %w", salesPath, err)
	}
	defer sf.Close()

	return ReadCSV(ctx, cf, sf)
}

// ReadCSV parses and validates clients.csv and sales.csv (v4 6-col or v5 7-col with product_group)
// while preserving source order.
func ReadCSV(ctx context.Context, clientsR, salesR io.Reader) (Dataset, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if clientsR == nil {
		return Dataset{}, errors.New("clients reader is required")
	}
	if salesR == nil {
		return Dataset{}, errors.New("sales reader is required")
	}

	clients, clientSet, err := readClientsCSV(ctx, clientsR)
	if err != nil {
		return Dataset{}, err
	}
	sales, productGroups, hasGroupCol, err := readSalesCSV(ctx, salesR, clientSet)
	if err != nil {
		return Dataset{}, err
	}
	return Dataset{
		Clients:       clients,
		Sales:         sales,
		ProductGroups: productGroups,
		HasGroupCol:   hasGroupCol,
	}, nil
}

// ImportCSV validates CSV streams and publishes ordered batches to writer.
// Validation completes before WriteDataset is called so invalid inputs never publish partial batches.
func ImportCSV(
	ctx context.Context,
	version string,
	clientsR, salesR io.Reader,
	writer contract.DatasetWriter,
	batchSize int,
) (ImportSummary, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	version = strings.TrimSpace(version)
	if version == "" {
		return ImportSummary{}, errors.New("dataset version is required")
	}
	if writer == nil {
		return ImportSummary{}, errors.New("dataset writer is required")
	}

	ds, err := ReadCSV(ctx, clientsR, salesR)
	if err != nil {
		return ImportSummary{}, err
	}
	batches := BuildBatches(ds, batchSize)
	if err := writer.WriteDataset(ctx, version, batches); err != nil {
		return ImportSummary{}, fmt.Errorf("write dataset version %q: %w", version, err)
	}

	items := make(map[domain.ItemID]struct{})
	pgroups := make(map[string]struct{})
	emptySKU := 0
	for _, s := range ds.Sales {
		items[s.ItemID] = struct{}{}
		pg := ds.ProductGroups[s.ItemID]
		if pg == "" {
			pg = string(s.ItemID)
		}
		pgroups[pg] = struct{}{}
		if s.SKU == nil {
			emptySKU++
		}
	}
	return ImportSummary{
		Version:               version,
		Clients:               len(ds.Clients),
		Sales:                 len(ds.Sales),
		Batches:               len(batches),
		DistinctItems:         len(items),
		DistinctProductGroups: len(pgroups),
		EmptySKURows:          emptySKU,
		HasProductGroupColumn: ds.HasGroupCol,
	}, nil
}

// BuildBatches splits Dataset into ordered DatasetBatch slices with clients before sales.
func BuildBatches(ds Dataset, batchSize int) []contract.DatasetBatch {
	if batchSize <= 0 {
		batchSize = DefaultBatchSize
	}
	batches := make([]contract.DatasetBatch, 0, (len(ds.Clients)+batchSize-1)/batchSize+(len(ds.Sales)+batchSize-1)/batchSize)
	for start := 0; start < len(ds.Clients); start += batchSize {
		end := min(start+batchSize, len(ds.Clients))
		batches = append(batches, contract.DatasetBatch{
			Clients: slices.Clone(ds.Clients[start:end]),
		})
	}
	for start := 0; start < len(ds.Sales); start += batchSize {
		end := min(start+batchSize, len(ds.Sales))
		batches = append(batches, contract.DatasetBatch{
			Sales: slices.Clone(ds.Sales[start:end]),
		})
	}
	return batches
}

// VerifyV4Release validates that version, SHA-256, and dataset sizes match the closed v4 release.
func VerifyV4Release(version, workbookSHA256 string, salesCount, itemCount int) error {
	return verifyRelease(ExpectedV4Version, ExpectedV4WorkbookSHA256, version, workbookSHA256, salesCount, itemCount)
}

// VerifyV5Release validates that version, SHA-256, and dataset sizes match the closed v5 release.
func VerifyV5Release(version, workbookSHA256 string, salesCount, itemCount, productGroupCount int) error {
	if err := verifyRelease(ExpectedV5Version, ExpectedV5WorkbookSHA256, version, workbookSHA256, salesCount, itemCount); err != nil {
		return err
	}
	if productGroupCount != ExpectedV5ProductGroups {
		return fmt.Errorf("unexpected v5 product_group count %d, want %d", productGroupCount, ExpectedV5ProductGroups)
	}
	return nil
}

func verifyRelease(wantVer, wantSHA, version, workbookSHA256 string, salesCount, itemCount int) error {
	if strings.TrimSpace(version) != wantVer {
		return fmt.Errorf("unexpected dataset version %q, want %q", version, wantVer)
	}
	if !strings.EqualFold(strings.TrimSpace(workbookSHA256), wantSHA) {
		return fmt.Errorf("unexpected %s workbook sha256 %q, want %q", wantVer, workbookSHA256, wantSHA)
	}
	if salesCount != ExpectedV4Sales || itemCount != ExpectedV4Items {
		return fmt.Errorf("unexpected %s sizes: %d sales, %d items (want %d sales, %d items)",
			wantVer, salesCount, itemCount, ExpectedV4Sales, ExpectedV4Items)
	}
	return nil
}

func readClientsCSV(ctx context.Context, r io.Reader) ([]domain.Client, map[domain.ClientID]struct{}, error) {
	reader := newCSVReader(r)
	header, err := reader.Read()
	if err != nil {
		return nil, nil, fmt.Errorf("read clients header: %w", err)
	}
	stripBOM(header)
	if err := validateHeader("clients.csv", header, expectedClientsHeader); err != nil {
		return nil, nil, err
	}

	var clients []domain.Client
	seen := make(map[domain.ClientID]struct{})
	for line := 2; ; line++ {
		if err := ctx.Err(); err != nil {
			return nil, nil, fmt.Errorf("read clients.csv line %d: %w", line, err)
		}
		record, err := reader.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, nil, fmt.Errorf("read clients.csv line %d: %w", line, err)
		}
		if len(record) != len(expectedClientsHeader) {
			return nil, nil, fmt.Errorf("clients.csv line %d: got %d columns, want %d", line, len(record), len(expectedClientsHeader))
		}

		id := domain.ClientID(strings.TrimSpace(record[0]))
		if id == "" {
			return nil, nil, fmt.Errorf("clients.csv line %d: empty client_id", line)
		}
		if _, dup := seen[id]; dup {
			return nil, nil, fmt.Errorf("clients.csv line %d: duplicate client_id %q", line, id)
		}
		seen[id] = struct{}{}

		group, err := parseClientGroup(record[1])
		if err != nil {
			return nil, nil, fmt.Errorf("clients.csv line %d: %w", line, err)
		}
		lkAccess, err := parseLKAccess(record[5])
		if err != nil {
			return nil, nil, fmt.Errorf("clients.csv line %d: %w", line, err)
		}

		clients = append(clients, domain.Client{
			ID:          id,
			Group:       group,
			Type:        strings.TrimSpace(record[2]),
			Industry:    optionalString(record[3]),
			Segment:     optionalString(record[4]),
			HasLKAccess: lkAccess,
		})
	}
	return clients, seen, nil
}

func readSalesCSV(
	ctx context.Context,
	r io.Reader,
	knownClients map[domain.ClientID]struct{},
) ([]domain.Sale, map[domain.ItemID]string, bool, error) {
	reader := newCSVReader(r)
	header, err := reader.Read()
	if err != nil {
		return nil, nil, false, fmt.Errorf("read sales header: %w", err)
	}
	stripBOM(header)

	hasGroupCol := false
	wantCols := len(expectedSalesHeaderV4)
	switch len(header) {
	case len(expectedSalesHeaderV4):
		if err := validateHeader("sales.csv", header, expectedSalesHeaderV4); err != nil {
			return nil, nil, false, err
		}
	case len(expectedSalesHeaderV5):
		if err := validateHeader("sales.csv", header, expectedSalesHeaderV5); err != nil {
			return nil, nil, false, err
		}
		hasGroupCol = true
		wantCols = len(expectedSalesHeaderV5)
	default:
		return nil, nil, false, fmt.Errorf("sales.csv: header has %d columns %v, want %v (v4) or %v (v5)",
			len(header), header, expectedSalesHeaderV4, expectedSalesHeaderV5)
	}

	var sales []domain.Sale
	productGroups := make(map[domain.ItemID]string)
	for line := 2; ; line++ {
		if err := ctx.Err(); err != nil {
			return nil, nil, false, fmt.Errorf("read sales.csv line %d: %w", line, err)
		}
		record, err := reader.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, nil, false, fmt.Errorf("read sales.csv line %d: %w", line, err)
		}
		if len(record) != wantCols {
			return nil, nil, false, fmt.Errorf("sales.csv line %d: got %d columns, want %d", line, len(record), wantCols)
		}

		orderID := strings.TrimSpace(record[0])
		if orderID == "" {
			return nil, nil, false, fmt.Errorf("sales.csv line %d: empty order_id", line)
		}
		clientID := domain.ClientID(strings.TrimSpace(record[1]))
		if clientID == "" {
			return nil, nil, false, fmt.Errorf("sales.csv line %d: empty client_id", line)
		}
		if _, ok := knownClients[clientID]; !ok {
			return nil, nil, false, fmt.Errorf("sales.csv line %d: unknown client_id %q", line, clientID)
		}

		weekStr := strings.TrimSpace(record[2])
		orderWeek, err := time.Parse("2006-01-02", weekStr)
		if err != nil {
			return nil, nil, false, fmt.Errorf("sales.csv line %d: invalid order_week %q: %w", line, weekStr, err)
		}

		itemID := domain.ItemID(strings.TrimSpace(record[3]))
		if itemID == "" {
			return nil, nil, false, fmt.Errorf("sales.csv line %d: empty item_id", line)
		}

		pg := string(itemID)
		if hasGroupCol {
			if rawPG := strings.TrimSpace(record[6]); rawPG != "" {
				pg = rawPG
			}
		}
		if prevPG, exists := productGroups[itemID]; exists && prevPG != pg {
			return nil, nil, false, fmt.Errorf("sales.csv line %d: conflicting product_group %q for item_id %q (previous %q)",
				line, pg, itemID, prevPG)
		}
		productGroups[itemID] = pg

		sales = append(sales, domain.Sale{
			SourceRow: line,
			OrderID:   orderID,
			ClientID:  clientID,
			OrderWeek: orderWeek.UTC(),
			ItemID:    itemID,
			SKU:       optionalString(record[4]),
			ItemName:  strings.TrimSpace(record[5]),
		})
	}
	return sales, productGroups, hasGroupCol, nil
}

func newCSVReader(r io.Reader) *csv.Reader {
	reader := csv.NewReader(r)
	reader.FieldsPerRecord = -1
	reader.ReuseRecord = false
	return reader
}

func stripBOM(fields []string) {
	if len(fields) > 0 {
		fields[0] = strings.TrimPrefix(fields[0], "\ufeff")
	}
}

func validateHeader(file string, got, want []string) error {
	if len(got) != len(want) {
		return fmt.Errorf("%s: header has %d columns %v, want %v", file, len(got), got, want)
	}
	for i := range want {
		if strings.TrimSpace(got[i]) != want[i] {
			return fmt.Errorf("%s: header column %d is %q, want %q", file, i, got[i], want[i])
		}
	}
	return nil
}

func parseClientGroup(raw string) (domain.ClientGroup, error) {
	switch domain.ClientGroup(strings.TrimSpace(raw)) {
	case domain.B2B:
		return domain.B2B, nil
	case domain.B2C:
		return domain.B2C, nil
	default:
		return "", fmt.Errorf("invalid client_group %q (want B2B or B2C)", raw)
	}
}

func parseLKAccess(raw string) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "да", "true", "1", "yes":
		return true, nil
	case "нет", "false", "0", "no":
		return false, nil
	default:
		return false, fmt.Errorf("invalid lk_access %q", raw)
	}
}

func optionalString(raw string) *string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}
