package data

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"recsys/internal/contract"
	"recsys/internal/domain"
)

type memoryDatasetWriter struct {
	calls   int
	version string
	batches []contract.DatasetBatch
	err     error
}

func (m *memoryDatasetWriter) WriteDataset(ctx context.Context, version string, batches []contract.DatasetBatch) error {
	m.calls++
	if m.err != nil {
		return m.err
	}
	m.version = version
	m.batches = append([]contract.DatasetBatch(nil), batches...)
	return nil
}

func TestImportSmokeFixture(t *testing.T) {
	ctx := context.Background()
	clientsPath := filepath.Join("..", "..", "testdata", "smoke", "clients.csv")
	salesPath := filepath.Join("..", "..", "testdata", "smoke", "sales.csv")

	ds, err := LoadCSV(ctx, clientsPath, salesPath)
	if err != nil {
		t.Fatalf("LoadCSV failed: %v", err)
	}
	if len(ds.Clients) != 2 || len(ds.Sales) != 1 {
		t.Fatalf("unexpected smoke counts: clients=%d sales=%d", len(ds.Clients), len(ds.Sales))
	}
	if ds.Clients[0].ID != "DEMO-BUYER" || ds.Clients[0].Group != domain.B2B || !ds.Clients[0].HasLKAccess {
		t.Fatalf("unexpected first client: %+v", ds.Clients[0])
	}
	if ds.Clients[0].Industry == nil || *ds.Clients[0].Industry != "Образование" || ds.Clients[0].Segment != nil {
		t.Fatalf("unexpected first client attributes: %+v", ds.Clients[0])
	}
	if ds.Clients[1].ID != "DEMO-NEW" || ds.Clients[1].Group != domain.B2C || !ds.Clients[1].HasLKAccess {
		t.Fatalf("unexpected second client: %+v", ds.Clients[1])
	}
	sale := ds.Sales[0]
	if sale.SourceRow != 2 || sale.OrderID != "DEMO-ORDER-1" || sale.ClientID != "DEMO-BUYER" ||
		sale.ItemID != "DEMO-BASE" || sale.SKU == nil || *sale.SKU != "DEMO-BASE" ||
		sale.OrderWeek.Format("2006-01-02") != "2026-08-31" {
		t.Fatalf("unexpected smoke sale: %+v", sale)
	}

	writer := &memoryDatasetWriter{}
	batches := BuildBatches(ds, 1)
	if len(batches) != 3 {
		t.Fatalf("expected 3 single-row batches (2 client batches + 1 sale batch), got %d", len(batches))
	}
	if err := writer.WriteDataset(ctx, "demo-smoke", batches); err != nil {
		t.Fatalf("WriteDataset failed: %v", err)
	}
	if writer.version != "demo-smoke" || len(writer.batches) != 3 {
		t.Fatalf("unexpected writer state: version=%q batches=%d", writer.version, len(writer.batches))
	}
}

func TestImportCSVPreservesOrderEmptySKUAndNoOffer(t *testing.T) {
	clientsCSV := "\ufeffclient_id,client_group,client_type,industry,segment,lk_access\n" +
		"C2,B2B,ЮЛ,IT,mid,нет\n" +
		"C1,B2C,ФЛ,,,да\n"
	salesCSV := "\ufefforder_id,client_id,order_week,item_id,sku,item_name\n" +
		"O2,C2,2026-08-31,N-abc,,Товар без SKU\n" +
		"O1,C1,2026-09-07,UNMATCHED-SKU,UNMATCHED-SKU,Товар без оффера\n"

	writer := &memoryDatasetWriter{}
	summary, err := ImportCSV(
		context.Background(),
		"demo-v1",
		strings.NewReader(clientsCSV),
		strings.NewReader(salesCSV),
		writer,
		1,
	)
	if err != nil {
		t.Fatalf("ImportCSV failed: %v", err)
	}
	if summary.Clients != 2 || summary.Sales != 2 || summary.Batches != 4 || summary.EmptySKURows != 1 || summary.DistinctItems != 2 {
		t.Fatalf("unexpected summary: %+v", summary)
	}
	if writer.batches[0].Clients[0].ID != "C2" || writer.batches[1].Clients[0].ID != "C1" {
		t.Fatalf("client order not preserved: %+v %+v", writer.batches[0].Clients, writer.batches[1].Clients)
	}
	s0 := writer.batches[2].Sales[0]
	s1 := writer.batches[3].Sales[0]
	if s0.SourceRow != 2 || s0.SKU != nil || s0.ItemID != "N-abc" {
		t.Fatalf("expected empty SKU and SourceRow=2 preserved, got %+v", s0)
	}
	if s1.SourceRow != 3 || s1.SKU == nil || *s1.SKU != "UNMATCHED-SKU" {
		t.Fatalf("expected second sale preserved, got %+v", s1)
	}
}

func TestImportCSVV5ProductGroupColumn(t *testing.T) {
	clientsCSV := "client_id,client_group,client_type,industry,segment,lk_access\n" +
		"C1,B2B,ЮЛ,IT,mid,да\n" +
		"C2,B2C,ФЛ,,,нет\n"
	salesCSV := "order_id,client_id,order_week,item_id,sku,item_name,product_group\n" +
		"O1,C1,2026-08-31,SKU-1,SKU-1,Product Base,G100\n" +
		"O2,C1,2026-09-07,SKU-2,SKU-2,Product Renew,G100\n" +
		"O3,C2,2026-09-07,N-abc,,Product NoGroup,\n"

	writer := &memoryDatasetWriter{}
	summary, err := ImportCSV(
		context.Background(),
		"v5",
		strings.NewReader(clientsCSV),
		strings.NewReader(salesCSV),
		writer,
		10,
	)
	if err != nil {
		t.Fatalf("ImportCSV v5 failed: %v", err)
	}
	if !summary.HasProductGroupColumn || summary.DistinctItems != 3 || summary.DistinctProductGroups != 2 {
		t.Fatalf("unexpected v5 summary: %+v", summary)
	}

	ds, err := ReadCSV(context.Background(), strings.NewReader(clientsCSV), strings.NewReader(salesCSV))
	if err != nil {
		t.Fatalf("ReadCSV v5 failed: %v", err)
	}
	if ds.ProductGroups["SKU-1"] != "G100" || ds.ProductGroups["SKU-2"] != "G100" || ds.ProductGroups["N-abc"] != "N-abc" {
		t.Fatalf("unexpected product_group mapping: %+v", ds.ProductGroups)
	}

	conflictSales := "order_id,client_id,order_week,item_id,sku,item_name,product_group\n" +
		"O1,C1,2026-08-31,SKU-1,SKU-1,Product Base,G100\n" +
		"O2,C2,2026-09-07,SKU-1,SKU-1,Product Base,G200\n"
	if _, err := ReadCSV(context.Background(), strings.NewReader(clientsCSV), strings.NewReader(conflictSales)); err == nil {
		t.Fatalf("expected error for conflicting product_group on same item_id")
	}

	badHeaderSales := "order_id,client_id,order_week,item_id,sku,item_name,family\n" +
		"O1,C1,2026-08-31,SKU-1,SKU-1,Product Base,G100\n"
	if _, err := ReadCSV(context.Background(), strings.NewReader(clientsCSV), strings.NewReader(badHeaderSales)); err == nil {
		t.Fatalf("expected error for unexpected 7th column header")
	}
}

func TestImportCSVValidationAndFailClosed(t *testing.T) {
	validClients := "client_id,client_group,client_type,industry,segment,lk_access\nC1,B2B,ЮЛ,,,да\n"
	validSales := "order_id,client_id,order_week,item_id,sku,item_name\nO1,C1,2026-08-31,I1,,Item\n"

	cases := []struct {
		name    string
		clients string
		sales   string
	}{
		{
			name:    "bad clients header",
			clients: "id,client_group,client_type,industry,segment,lk_access\nC1,B2B,ЮЛ,,,да\n",
			sales:   validSales,
		},
		{
			name:    "bad sales header",
			clients: validClients,
			sales:   "order_id,client_id,week,item_id,sku,item_name\nO1,C1,2026-08-31,I1,,Item\n",
		},
		{
			name:    "unknown client_id in sales",
			clients: validClients,
			sales:   "order_id,client_id,order_week,item_id,sku,item_name\nO1,MISSING,2026-08-31,I1,,Item\n",
		},
		{
			name:    "invalid client_group",
			clients: "client_id,client_group,client_type,industry,segment,lk_access\nC1,GOV,ЮЛ,,,да\n",
			sales:   validSales,
		},
		{
			name:    "invalid order_week",
			clients: validClients,
			sales:   "order_id,client_id,order_week,item_id,sku,item_name\nO1,C1,31.08.2026,I1,,Item\n",
		},
		{
			name:    "duplicate client_id",
			clients: validClients + "C1,B2B,ЮЛ,,,нет\n",
			sales:   validSales,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			writer := &memoryDatasetWriter{}
			_, err := ImportCSV(context.Background(), "demo-v1", strings.NewReader(tc.clients), strings.NewReader(tc.sales), writer, 10)
			if err == nil {
				t.Fatalf("expected error for %s", tc.name)
			}
			if writer.calls != 0 {
				t.Fatalf("DatasetWriter must not be called on validation error (called %d times)", writer.calls)
			}
		})
	}

	t.Run("writer error propagates", func(t *testing.T) {
		writer := &memoryDatasetWriter{err: errors.New("db unavailable")}
		_, err := ImportCSV(context.Background(), "demo-v1", strings.NewReader(validClients), strings.NewReader(validSales), writer, 10)
		if err == nil || !strings.Contains(err.Error(), "db unavailable") {
			t.Fatalf("expected wrapped writer error, got %v", err)
		}
	})

	t.Run("cancelled context stops read", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		writer := &memoryDatasetWriter{}
		_, err := ImportCSV(ctx, "demo-v1", strings.NewReader(validClients), strings.NewReader(validSales), writer, 10)
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("expected context.Canceled, got %v", err)
		}
	})
}

func TestCheckAnonymization(t *testing.T) {
	clients := []domain.Client{
		{ID: "B1", Group: domain.B2B},
		{ID: "B2", Group: domain.B2B},
		{ID: "B3", Group: domain.B2B},
		{ID: "F1", Group: domain.B2C},
		{ID: "F2", Group: domain.B2C},
		{ID: "F3", Group: domain.B2C},
		{ID: "F4", Group: domain.B2C},
		{ID: "F5", Group: domain.B2C},
	}

	t.Run("valid thresholds", func(t *testing.T) {
		sales := []domain.Sale{
			{SourceRow: 2, ClientID: "B1", ItemID: "B2B-ITEM"},
			{SourceRow: 3, ClientID: "B2", ItemID: "B2B-ITEM"},
			{SourceRow: 4, ClientID: "B3", ItemID: "B2B-ITEM"},
			{SourceRow: 5, ClientID: "F1", ItemID: "MIX-ITEM"},
			{SourceRow: 6, ClientID: "F2", ItemID: "MIX-ITEM"},
			{SourceRow: 7, ClientID: "F3", ItemID: "MIX-ITEM"},
			{SourceRow: 8, ClientID: "F4", ItemID: "MIX-ITEM"},
			{SourceRow: 9, ClientID: "F5", ItemID: "MIX-ITEM"},
		}
		rep, err := CheckAnonymization(clients, sales)
		if err != nil {
			t.Fatalf("expected valid anonymization, got %v", err)
		}
		if rep.MinItemBuyers != 3 || rep.MinB2CBuyers != 5 {
			t.Fatalf("unexpected min counts: %+v", rep)
		}
	})

	t.Run("repeat buyer counts once", func(t *testing.T) {
		sales := []domain.Sale{
			{SourceRow: 2, ClientID: "B1", ItemID: "LOW-TOTAL"},
			{SourceRow: 3, ClientID: "B1", ItemID: "LOW-TOTAL"},
			{SourceRow: 4, ClientID: "B2", ItemID: "LOW-TOTAL"},
		}
		if _, err := CheckAnonymization(clients, sales); err == nil {
			t.Fatalf("expected violation for only 2 distinct buyers across 3 rows")
		}
	})

	t.Run("b2c threshold enforced even with enough b2b buyers", func(t *testing.T) {
		sales := []domain.Sale{
			{SourceRow: 2, ClientID: "B1", ItemID: "LOW-B2C"},
			{SourceRow: 3, ClientID: "B2", ItemID: "LOW-B2C"},
			{SourceRow: 4, ClientID: "F1", ItemID: "LOW-B2C"},
			{SourceRow: 5, ClientID: "F2", ItemID: "LOW-B2C"},
			{SourceRow: 6, ClientID: "F3", ItemID: "LOW-B2C"},
			{SourceRow: 7, ClientID: "F4", ItemID: "LOW-B2C"},
		}
		if _, err := CheckAnonymization(clients, sales); err == nil {
			t.Fatalf("expected violation for B2C item with 4 B2C buyers (< 5)")
		}
	})
}
