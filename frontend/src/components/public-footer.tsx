type PublicFooterProps = {
  showAgreement?: boolean;
};

const linkClassName = 'underline hover:text-gray-700 dark:hover:text-gray-300';

export function PublicFooter({ showAgreement = false }: PublicFooterProps) {
  return (
    <footer className="mt-6 max-w-md px-4 text-center text-xs text-gray-500 dark:text-gray-400">
      {showAgreement ? (
        <p>
          By signing in, you agree to our{' '}
          <a href="/terms" className={linkClassName}>
            Terms of Service
          </a>{' '}
          and{' '}
          <a href="/privacy" className={linkClassName}>
            Privacy Policy
          </a>
          .
        </p>
      ) : (
        <nav aria-label="Legal">
          <a href="/terms" className={linkClassName}>
            Terms of Service
          </a>
          <span aria-hidden="true"> · </span>
          <a href="/privacy" className={linkClassName}>
            Privacy Policy
          </a>
        </nav>
      )}
      <p className="mt-2">Unofficial unit tool — not a MINDEF or SAF system.</p>
    </footer>
  );
}
