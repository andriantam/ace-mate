// The app mark: a white gate on a black tile. Same drawing as public/favicon.svg.
export function Logo({ size = 22 }: { size?: number }) {
  return (
    <svg className="logo" width={size} height={size} viewBox="0 0 100 100" aria-hidden="true">
      <rect width="100" height="100" rx="22" fill="#000" />
      <path d="M22 29C24 31.6 31 33 40.5 33H59.5C69 33 76 31.6 78 29V38H59.3V74.6H54.9V38H45.1V74.6H40.7V38H22Z" fill="#fff" />
    </svg>
  );
}
