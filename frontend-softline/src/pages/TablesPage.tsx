import type { ReactNode } from "react";
import {
  INVOICES,
  INVOICE_STATUS,
  KEYS,
  KEY_STATUS,
  NOTIFICATIONS,
  NOTIFICATION_STATUS,
  ORDERS,
  STATUS_LABELS,
  findUser,
} from "../data/account";
import { formatPrice } from "../lib/format";
import { useSettings } from "../state/settings";

interface Column<T> {
  key: string;
  label: string;
  render?: (row: T) => ReactNode;
}

interface TablePageProps<T> {
  title: string;
  columns: Array<Column<T>>;
  rows: T[];
  rowKey: (row: T) => string;
}

function TablePage<T>({ title, columns, rows, rowKey }: TablePageProps<T>) {
  return (
    <div className="react-page-inner">
      <h1>{title}</h1>
      <p className="react-page__lead">Тестовые данные локального клона личного кабинета.</p>
      <div className="clone-card">
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
  return (
    <TablePage
      title="Мои заказы"
      columns={[
        { key: "id", label: "Номер" },
        { key: "date", label: "Дата" },
        { key: "title", label: "Продукт" },
        { key: "vendor", label: "Вендор" },
        { key: "sum", label: "Сумма", render: (row) => formatPrice(row.sum) },
        {
          key: "status",
          label: "Статус",
          render: (row) => {
            const status = STATUS_LABELS[row.status];
            return <span className={status.className}>{status.label}</span>;
          },
        },
      ]}
      rows={ORDERS}
      rowKey={(row) => row.id}
    />
  );
}

export function InvoicesPage() {
  return (
    <TablePage
      title="Счета и платежи"
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
  const { settings } = useSettings();
  const user = findUser(settings.userId);
  return (
    <div className="react-page-inner">
      <h1>{title}</h1>
      <p className="react-page__lead">{text}</p>
      <div className="clone-card">
        <p>
          <b>{user.name}</b>
        </p>
        <p>{user.email}</p>
        <p>История покупок: {user.history.join(", ")}</p>
      </div>
    </div>
  );
}
