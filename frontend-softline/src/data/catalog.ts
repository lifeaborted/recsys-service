/** Каталог из снимка lk.softline.ru — underpinning для mock-рекомендаций. */
export interface CatalogItem {
  id: string;
  sku: string;
  title: string;
  subtitle: string;
  vendor: string;
  category: string;
  price: number;
  image: string;
  tags: string[];
  url: string;
  popularity: number;
  demoRank: number;
}

export const CATEGORIES = [
  "Безопасность",
  "Офисное ПО",
  "Аналитика",
  "Операционные системы",
] as const;

export const CATALOG: CatalogItem[] = [
  {
    id: "p001", sku: "KL4313RARFS",
    title: "Kaspersky Security для почтовых серверов. Базовая",
    subtitle: "«Лаборатория Касперского»", vendor: "«Лаборатория Касперского»",
    category: "Безопасность", price: 1512,
    image: "/assets/scale-dec-168x168.png",
    tags: ["антивирус", "почта", "сервер", "базовая", "лицензия"],
    url: "https://store.softline.ru/kaspersky/kl4313rarfs-3851/",
    popularity: 86, demoRank: 1,
  },
  {
    id: "p002", sku: "PRO32-PUS-NS-EKEY-2-3",
    title: "PRO32 Office Security Base лицензия на 2 года и 3 устройства",
    subtitle: "PRO32", vendor: "PRO32",
    category: "Безопасность", price: 3090,
    image: "/assets/scale-dec-168x168(1).png",
    tags: ["office", "лицензия", "2 года", "3 устройства", "защита"],
    url: "https://store.softline.ru/pro32/pro32-pus-ns-ekey-2-3-518958/",
    popularity: 92, demoRank: 2,
  },
  {
    id: "p003", sku: "CR16-BDS1",
    title: "ContentReader PDF 16, Business 1 Standalone (подписка на 1 год)",
    subtitle: "Content AI", vendor: "Content AI (ООО «Контент ИИ», ранее ABBYY Россия)",
    category: "Офисное ПО", price: 16800,
    image: "/assets/scale-dec-168x168.jpg",
    tags: ["pdf", "редактор", "abbyy", "подписка", "бизнес"],
    url: "https://store.softline.ru/contentai/cr16-bds1-525823/",
    popularity: 61, demoRank: 3,
  },
  {
    id: "p004", sku: "PRO32-OSB-EKEY-1Y-5",
    title: "PRO32 Office Security Base лицензия на 1 год и на 5 устройств",
    subtitle: "PRO32", vendor: "PRO32",
    category: "Безопасность", price: 2490,
    image: "/assets/scale-dec-168x168(1).png",
    tags: ["office", "лицензия", "1 год", "5 устройств", "защита"],
    url: "https://store.softline.ru/pro32/pro32-osb-ekey-1y-5-ns-ru-aks-446656/",
    popularity: 89, demoRank: 4,
  },
  {
    id: "p005", sku: "KL4155RARFS",
    title: "Kaspersky Security для виртуальных и облачных сред. Desktop. Базовая",
    subtitle: "«Лаборатория Касперского»", vendor: "«Лаборатория Касперского»",
    category: "Безопасность", price: 2889.9,
    image: "/assets/scale-dec-168x168(2).png",
    tags: ["антивирус", "виртуализация", "desktop", "облако", "базовая"],
    url: "https://store.softline.ru/kaspersky/kl4155rarfs-160801/",
    popularity: 77, demoRank: 5,
  },
  {
    id: "p006", sku: "PRO32-OSB-EKEY-1Y-10",
    title: "PRO32 Office Security Base лицензия на 1 год и на 10 устройств",
    subtitle: "PRO32", vendor: "PRO32",
    category: "Безопасность", price: 4990,
    image: "/assets/scale-dec-168x168(1).png",
    tags: ["office", "лицензия", "1 год", "10 устройств", "защита"],
    url: "https://store.softline.ru/pro32/pro32-osb-ekey-1y-10-ns-ru-aks-446657/",
    popularity: 84, demoRank: 6,
  },
  {
    id: "p007", sku: "KL4255RARFS",
    title: "Kaspersky Security для виртуальных и облачных сред. Server. Базовая",
    subtitle: "«Лаборатория Касперского»", vendor: "«Лаборатория Касперского»",
    category: "Безопасность", price: 10212.3,
    image: "/assets/scale-dec-168x168(2).png",
    tags: ["антивирус", "виртуализация", "server", "облако", "базовая"],
    url: "https://store.softline.ru/kaspersky/kl4255rarfs-160810/",
    popularity: 73, demoRank: 7,
  },
  {
    id: "p008", sku: "KL4413RARFS",
    title: "Kaspersky Security для интернет-шлюзов. Базовая",
    subtitle: "«Лаборатория Касперского»", vendor: "«Лаборатория Касперского»",
    category: "Безопасность", price: 1034.37,
    image: "/assets/scale-dec-168x168(3).png",
    tags: ["антивирус", "шлюз", "сеть", "базовая", "лицензия"],
    url: "https://store.softline.ru/kaspersky/kl4413rarfs-3939/",
    popularity: 68, demoRank: 8,
  },
  {
    id: "p009", sku: "PRO32-OSB-EKEY-2Y-10",
    title: "PRO32 Office Security Base лицензия на 2 года и на 10 устройств",
    subtitle: "PRO32", vendor: "PRO32",
    category: "Безопасность", price: 9980,
    image: "/assets/scale-dec-168x168(1).png",
    tags: ["office", "лицензия", "2 года", "10 устройств", "защита"],
    url: "https://store.softline.ru/pro32/pro32-osb-ekey-2y-10-ns-ru-aks-536429/",
    popularity: 81, demoRank: 9,
  },
  {
    id: "p010", sku: "DATALENS",
    title: "Yandex DataLens — доступная бизнес-аналитика",
    subtitle: "Яндекс", vendor: "Яндекс",
    category: "Аналитика", price: 0,
    image: "/assets/scale-dec-168x168.jpg",
    tags: ["bi", "аналитика", "дашборды", "облако"],
    url: "https://store.softline.ru/",
    popularity: 55, demoRank: 99,
  },
  {
    id: "p011", sku: "ASTRA-LINUX",
    title: "Astra Linux — выгодное предложение для компаний до 500 сотрудников",
    subtitle: "Астра", vendor: "Астра",
    category: "Операционные системы", price: 0,
    image: "/assets/scale-dec-168x168.jpg",
    tags: ["ос", "импортозамещение", "безопасность", "сервер"],
    url: "https://store.softline.ru/",
    popularity: 52, demoRank: 99,
  },
  {
    id: "p012", sku: "CONTENT-AI-SUITE",
    title: "Content AI — инструменты работы с документами",
    subtitle: "Content AI", vendor: "Content AI (ООО «Контент ИИ», ранее ABBYY Россия)",
    category: "Офисное ПО", price: 12500,
    image: "/assets/scale-dec-168x168.jpg",
    tags: ["pdf", "документы", "abbyy", "поиск", "бизнес"],
    url: "https://store.softline.ru/contentai/",
    popularity: 47, demoRank: 99,
  },
];

export function findProduct(id: string): CatalogItem | undefined {
  return CATALOG.find((item) => item.id === id || item.sku === id);
}
