import type { Scenario } from "../api/types";

export const SCENARIO_OPTIONS: Array<{ value: Scenario; label: string; hint: string }> = [
  {
    value: "demo",
    label: "Исходная выдача",
    hint: "Порядок как на lk.softline.ru — эталон для сравнения",
  },
  {
    value: "cross-sell",
    label: "Кросс-продажа",
    hint: "Дополнительные сервисы к текущему портфелю",
  },
  {
    value: "upsell",
    label: "Апгрейд",
    hint: "Переход на старший тариф или больший лимит",
  },
  {
    value: "onboarding",
    label: "Онбординг",
    hint: "Помощь на старте работы с платформой",
  },
  {
    value: "retention",
    label: "Удержание",
    hint: "Снижение риска оттока клиента",
  },
];

export function scenarioLabel(scenario: Scenario): string {
  return SCENARIO_OPTIONS.find((option) => option.value === scenario)?.label ?? scenario;
}
