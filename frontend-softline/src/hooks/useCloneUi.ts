import { useEffect, type RefObject } from "react";

function chevron(direction: "prev" | "next"): string {
  const path = direction === "prev" ? "M14 18L8 12L14 6" : "M9 18L15 12L9 6";
  return (
    '<svg class="icon icon-size-20 sl-color-icon button__icon" viewBox="0 0 24 24" aria-hidden="true">' +
    `<path d="${path}" fill="none" stroke="currentColor" stroke-width="2" ` +
    'stroke-linecap="round" stroke-linejoin="round"/></svg>'
  );
}

/**
 * Навешивает поведение на статическую разметку из снимка:
 * прокрутку слайдеров, выпадающие меню, закрытие алерта и мобильное меню.
 */
export function useCloneUi(rootRef: RefObject<HTMLElement | null>, routeKey: string): void {
  useEffect(() => {
    const root = rootRef.current;
    if (!root) return;

    const cleanups: Array<() => void> = [];

    root.querySelectorAll<HTMLElement>(".slider:not([data-clone-ready])").forEach((slider) => {
      slider.dataset.cloneReady = "1";
      const swiper = slider.querySelector<HTMLElement>(".swiper");
      if (!swiper) return;

      // У статичных слайдеров стрелки уже есть в разметке; у динамических — нет.
      if (!slider.querySelector(".slider__control")) {
        slider.insertAdjacentHTML(
          "afterbegin",
          '<div class="slider__control slider__control_prev" data-v-af5a7bd7="">' +
            `<button class="button button_default button_size-l button_circle" type="button">${chevron("prev")}</button>` +
            "</div>",
        );
        slider.insertAdjacentHTML(
          "beforeend",
          '<div class="slider__control slider__control_next" data-v-af5a7bd7="">' +
            `<button class="button button_default button_size-l button_circle" type="button">${chevron("next")}</button>` +
            "</div>",
        );
      }

      const step = (direction: number) => {
        const slide = swiper.querySelector<HTMLElement>(".swiper-slide");
        const width = slide ? slide.getBoundingClientRect().width + 16 : swiper.clientWidth * 0.8;
        swiper.scrollBy({ left: direction * width, behavior: "smooth" });
      };
      const prev = slider.querySelector<HTMLElement>(".slider__control_prev");
      const next = slider.querySelector<HTMLElement>(".slider__control_next");
      const onPrev = () => step(-1);
      const onNext = () => step(1);
      prev?.addEventListener("click", onPrev);
      next?.addEventListener("click", onNext);
      cleanups.push(() => {
        prev?.removeEventListener("click", onPrev);
        next?.removeEventListener("click", onNext);
      });
    });

    root.querySelectorAll<HTMLElement>(".alert__close-button").forEach((button) => {
      const onClose = () => button.closest(".alert-wrapper")?.remove();
      button.addEventListener("click", onClose);
      cleanups.push(() => button.removeEventListener("click", onClose));
    });

    const onDocumentClick = (event: MouseEvent) => {
      const target = event.target as HTMLElement | null;
      const trigger = target?.closest<HTMLElement>(".dropdown-wrapper > div > button");
      const wrapper = trigger?.closest<HTMLElement>(".dropdown-wrapper");
      document.querySelectorAll<HTMLElement>(".dropdown.is-active").forEach((dropdown) => {
        if (!wrapper || dropdown.parentElement !== wrapper) dropdown.classList.remove("is-active");
      });
      if (trigger && wrapper) {
        wrapper.querySelector<HTMLElement>(".dropdown")?.classList.toggle("is-active");
      }
    };
    document.addEventListener("click", onDocumentClick);
    cleanups.push(() => document.removeEventListener("click", onDocumentClick));

    const menuButton = root.querySelector<HTMLElement>(".b-store-header__menu-btn");
    const middle = root.querySelector<HTMLElement>(".b-store-header__middle");
    const onMenu = () => middle?.classList.toggle("is-open");
    menuButton?.addEventListener("click", onMenu);
    cleanups.push(() => menuButton?.removeEventListener("click", onMenu));

    return () => cleanups.forEach((cleanup) => cleanup());
  }, [rootRef, routeKey]);
}
