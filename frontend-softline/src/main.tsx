import { createRoot } from "react-dom/client";
import { App } from "./App";
import "./styles/clone.css";

const container = document.getElementById("root");
if (!container) {
  throw new Error("Не найден корневой элемент #root");
}

// StrictMode намеренно выключен: панель отладки обновляет провайдер из эффектов
// дочерних страниц, и в React 18 это даёт ложное предупреждение в dev-консоли.
createRoot(container).render(<App />);
