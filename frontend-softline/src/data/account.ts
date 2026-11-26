export interface SoftlineUser {
  id: string;
  name: string;
  email: string;
  avatar: string;
  history: string[];
  affinities: { category: Record<string, number>; vendor: Record<string, number> };
}

export const USERS: SoftlineUser[] = [
  {
    id: "u1", name: "Иван Иванов(Тест)", email: "471hattie@sharebot.net", avatar: "ИИ",
    history: ["p002", "p004", "p006"],
    affinities: {
      category: { "Безопасность": 0.95, "Офисное ПО": 0.45 },
      vendor: { "PRO32": 0.9, "«Лаборатория Касперского»": 0.75 },
    },
  },
  {
    id: "u2", name: "Мария Петрова", email: "maria.petrova@example.com", avatar: "МП",
    history: ["p003", "p012"],
    affinities: {
      category: { "Аналитика": 0.9, "Офисное ПО": 0.8 },
      vendor: { "Яндекс": 0.9, "Content AI (ООО «Контент ИИ», ранее ABBYY Россия)": 0.8 },
    },
  },
  {
    id: "u3", name: "Сергей Смирнов", email: "sergey.smirnov@example.com", avatar: "СС",
    history: ["p011", "p001"],
    affinities: {
      category: { "Операционные системы": 0.9, "Безопасность": 0.6 },
      vendor: { "Астра": 0.9, "«Лаборатория Касперского»": 0.6 },
    },
  },
];

export function findUser(id: string): SoftlineUser {
  return USERS.find((user) => user.id === id) ?? USERS[0];
}

export interface OrderRow {
  id: string;
  date: string;
  title: string;
  vendor: string;
  sum: number;
  status: "ok" | "progress";
}

export const ORDERS: OrderRow[] = [
  { id: "SO-24001", date: "28.11.2026", title: "PRO32 Office Security Base, 1 год / 5 устройств", vendor: "PRO32", sum: 2490, status: "ok" },
  { id: "SO-23987", date: "14.11.2026", title: "Kaspersky Security для почтовых серверов. Базовая", vendor: "«Лаборатория Касперского»", sum: 1512, status: "ok" },
  { id: "SO-23840", date: "02.11.2026", title: "ContentReader PDF 16, Business", vendor: "Content AI", sum: 16800, status: "progress" },
];

export const STATUS_LABELS: Record<OrderRow["status"], { label: string; className: string }> = {
  ok: { label: "Выполнен", className: "clone-status clone-status_ok" },
  progress: { label: "В обработке", className: "clone-status clone-status_warn" },
};

export interface InvoiceRow {
  id: string;
  date: string;
  order: string;
  sum: number;
  status: "paid" | "pending";
}

export const INVOICES: InvoiceRow[] = [
  { id: "СЧ-0001", date: "28.11.2026", order: "SO-24001", sum: 2490, status: "paid" },
  { id: "СЧ-0002", date: "14.11.2026", order: "SO-23987", sum: 1512, status: "paid" },
  { id: "СЧ-0003", date: "02.11.2026", order: "SO-23840", sum: 16800, status: "pending" },
];

export const INVOICE_STATUS: Record<InvoiceRow["status"], { label: string; className: string }> = {
  paid: { label: "Оплачен", className: "clone-status clone-status_ok" },
  pending: { label: "К оплате", className: "clone-status clone-status_warn" },
};

export interface KeyRow {
  id: string;
  product: string;
  license: string;
  until: string;
  status: "active" | "expiring";
}

export const KEYS: KeyRow[] = [
  { id: "KEY-001", product: "PRO32 Office Security Base", license: "1 год / 5 устройств", until: "28.11.2027", status: "active" },
  { id: "KEY-002", product: "Kaspersky Security для почтовых серверов", license: "1 год / 1 сервер", until: "14.11.2027", status: "active" },
  { id: "KEY-003", product: "ContentReader PDF 16, Business", license: "1 год / 1 АРМ", until: "02.11.2026", status: "expiring" },
];

export const KEY_STATUS: Record<KeyRow["status"], { label: string; className: string }> = {
  active: { label: "Активен", className: "clone-status clone-status_ok" },
  expiring: { label: "Истекает", className: "clone-status clone-status_warn" },
};

export interface NotificationRow {
  date: string;
  title: string;
  type: "warning" | "info" | "success";
}

export const NOTIFICATIONS: NotificationRow[] = [
  { date: "28.11.2026", title: "Ключ KEY-003 истекает через 5 дней", type: "warning" },
  { date: "26.11.2026", title: "Появились новые рекомендации для вашей компании", type: "info" },
  { date: "14.11.2026", title: "Счёт СЧ-0002 оплачен", type: "success" },
  { date: "02.11.2026", title: "Заказ SO-23840 передан в обработку", type: "info" },
];

export const NOTIFICATION_STATUS: Record<NotificationRow["type"], { label: string; className: string }> = {
  warning: { label: "Внимание", className: "clone-status clone-status_warn" },
  info: { label: "Информация", className: "clone-status clone-status_info" },
  success: { label: "Готово", className: "clone-status clone-status_ok" },
};
