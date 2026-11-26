interface FragmentProps {
  html: string;
  className?: string;
}

/** Статическая разметка из исходного снимка (шапка, сайдбар, виджеты). */
export function Fragment({ html, className }: FragmentProps) {
  return <div className={className} dangerouslySetInnerHTML={{ __html: html }} />;
}
