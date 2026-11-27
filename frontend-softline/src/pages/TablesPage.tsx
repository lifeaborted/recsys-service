import { useEffect, useState, type ReactNode } from "react";
import { listPurchases, type PurchaseHistoryItem } from "../api/recsysClient";
import { RECSYS_API, useSession } from "../state/session";
import {
  INVOICES,
  INVOICE_STATUS,
  KEYS,
  KEY_STATUS,
  NOTIFICATIONS,
  NOTIFICATION_STATUS,
} from "../data/account";
import { formatPrice } from "../lib/format";

interface Column<T> {
  key: string;
  label: string;
  render?: (row: T) => ReactNode;
}

interface TablePageProps<T> {
  title: string;
  lead: string;
  columns: Array<Column<T>>;
  rows: T[];
  rowKey: (row: T) => string;
}

function TablePage<T>({ title, lead, columns, rows, rowKey }: TablePageProps<T>) {
  return (
    <div className="react-page-inner">
      <h1>{title}</h1>
      <p className="react-page__lead">{lead}</p>
      <div className="clone-card account-table-scroll">
        <table className="clone-table">
          <thead>
            <tr>
              {columns.map((column) => (
                <th key={column.key}>{column.label}</th>
              ))}
            </tr>
          </thead>
          <tbody>
            {rows.map((row) => (
              <tr key={rowKey(row)}>
                {columns.map((column) => (
                  <td key={column.key}>
                    {column.render
                      ? column.render(row)
                      : String((row as Record<string, unknown>)[column.key] ?? "")}
                  </td>
                ))}
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
}

export function OrdersPage() {
  const { profile } = useSession();
  const token = profile?.token;
  const [rows, setRows] = useState<PurchaseHistoryItem[] | null>(null);
  const [error, setError] = useState("");
  useEffect(() => {
    if (!token) return;
    const controller = new AbortController();
    void listPurchases({ baseUrl: RECSYS_API, transport: "http", token }, controller.signal)
      .then((response) => { if (!controller.signal.aborted) setRows(response.items); })
      .catch((cause: unknown) => {
        if (!controller.signal.aborted) setError(cause instanceof Error ? cause.message : "Не удалось загрузить заказы.");
      });
    return () => controller.abort();
  }, [token]);

  if (error || !rows?.length) return (
    <div className="react-page-inner">
      <h1>Мои заказы</h1>
      <p role={error ? "alert" : "status"}>{error || (rows ? "В этой сессии пока нет заказов." : "Загружаем заказы…")}</p>
    </div>
  );
  return (
    <TablePage
      title="Мои заказы"
      lead="Тестовые покупки текущей сессии."
      columns={[
        { key: "order_id", label: "Номер заказа" },
        { key: "purchased_at", label: "Дата", render: (row) => new Date(row.purchased_at).toLocaleString("ru-RU") },
        { key: "title", label: "Продукт" },
        { key: "total", label: "Сумма", render: (row) => formatPrice(row.total, row.currency) },
      ]}
      rows={rows}
      rowKey={(row) => row.order_id}
    />
  );
}

export function InvoicesPage() {
  return (
    <TablePage
      title="Счета и платежи"
      lead="Демонстрационные счета. Тестовые покупки не создают платёжных документов."
      columns={[
        { key: "id", label: "Номер" },
        { key: "date", label: "Дата" },
        { key: "order", label: "Заказ" },
        { key: "sum", label: "Сумма", render: (row) => formatPrice(row.sum) },
        {
          key: "status",
          label: "Статус",
          render: (row) => {
            const status = INVOICE_STATUS[row.status];
            return <span className={status.className}>{status.label}</span>;
          },
        },
      ]}
      rows={INVOICES}
      rowKey={(row) => row.id}
    />
  );
}

export function KeysPage() {
  return (
    <TablePage
      title="Ключи и дистрибутивы"
      lead="Демонстрационные записи. Тестовые покупки не выпускают лицензии."
      columns={[
        { key: "id", label: "Ключ" },
        { key: "product", label: "Продукт" },
        { key: "license", label: "Лицензия" },
        { key: "until", label: "Действует до" },
        {
          key: "status",
          label: "Статус",
          render: (row) => {
            const status = KEY_STATUS[row.status];
            return <span className={status.className}>{status.label}</span>;
          },
        },
      ]}
      rows={KEYS}
      rowKey={(row) => row.id}
    />
  );
}

export function NotificationsPage() {
  return (
    <TablePage
      title="Уведомления"
      lead="Демонстрационные уведомления."
      columns={[
        { key: "date", label: "Дата" },
        { key: "title", label: "Уведомление" },
        {
          key: "type",
          label: "Тип",
          render: (row) => {
            const status = NOTIFICATION_STATUS[row.type];
            return <span className={status.className}>{status.label}</span>;
          },
        },
      ]}
      rows={NOTIFICATIONS}
      rowKey={(row) => row.date + row.title}
    />
  );
}

interface StaticPageProps {
  title: string;
  text: string;
}

export function StaticPage({ title, text }: StaticPageProps) {
  return (
    <div className="react-page-inner">
      <h1>{title}</h1>
      <p className="react-page__lead">{text}</p>
    </div>
  );
}
