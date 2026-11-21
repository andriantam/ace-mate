import { useEffect, useRef, useState } from "react";
import { Logo } from "./Logo";
import { Link, Navigate, NavLink, Route, Routes, useLocation, useNavigate } from "react-router-dom";
import { api } from "./api";
import { AddDocumentModal } from "./components/AddDocument";
import { DocsProvider, useDocs } from "./docs";
import { Account, Avatar } from "./pages/Account";
import { Consent } from "./pages/Consent";
import { Dashboard } from "./pages/Dashboard";
import { JobPage } from "./pages/JobPage";
import { Jobs } from "./pages/Jobs";
import { Landing } from "./pages/Landing";
import { Privacy } from "./pages/legal/Privacy";
import { Terms } from "./pages/legal/Terms";
import { NewJob } from "./pages/NewJob";
import { SignIn } from "./pages/SignIn";
import { useSession } from "./session";

// The main navigation: top-level pills, some with pages nested under them. Add a page here
// and it appears in the header on every screen size.
type NavItem = { to: string; label: string; match?: (path: string) => boolean; children?: NavItem[] };
const NAV: NavItem[] = [
  { to: "/dashboard", label: "Profile" },
  {
    to: "/jobs",
    label: "Jobs",
    match: (p) => p === "/jobs" || p.startsWith("/jobs/"),
    children: [
      { to: "/jobs", label: "Jobs Applied", match: (p) => p === "/jobs" || (p.startsWith("/jobs/") && p !== "/jobs/new") },
      { to: "/jobs/new", label: "Add Job Application" },
    ],
  },
];
const isActive = (n: NavItem, path: string) => (n.match ? n.match(path) : path === n.to);

/** A nav pill with nested pages in a dropdown. */
function NavGroup({ item }: { item: NavItem }) {
  const location = useLocation();
  const [open, setOpen] = useState(false);
  const ref = useRef<HTMLDivElement>(null);
  // Close the menu when the page changes (adjusting state during render, not in an effect).
  const [path, setPath] = useState(location.pathname);
  if (path !== location.pathname) {
    setPath(location.pathname);
    setOpen(false);
  }
  useEffect(() => {
    if (!open) return;
    const close = (e: MouseEvent) => !ref.current?.contains(e.target as Node) && setOpen(false);
    const esc = (e: KeyboardEvent) => e.key === "Escape" && setOpen(false);
    document.addEventListener("mousedown", close);
    document.addEventListener("keydown", esc);
    return () => {
      document.removeEventListener("mousedown", close);
      document.removeEventListener("keydown", esc);
    };
  }, [open]);
  return (
    <div className="navgroup" ref={ref}>
      <button
        type="button"
        className={`navpill ${isActive(item, location.pathname) ? "active" : ""}`}
        aria-haspopup="menu"
        aria-expanded={open}
        onClick={() => setOpen((o) => !o)}
      >
        {item.label} <span className="caret" aria-hidden="true">▾</span>
      </button>
      {open ? (
        <div className="menu navmenu" role="menu">
          {item.children!.map((c) => (
            <NavLink key={c.to} to={c.to} end role="menuitem" className={() => (isActive(c, location.pathname) ? "active" : "")}>
              {c.label}
            </NavLink>
          ))}
        </div>
      ) : null}
    </div>
  );
}

function UserMenu() {
  const { session, refresh } = useSession();
  const navigate = useNavigate();
  const [open, setOpen] = useState(false);
  const ref = useRef<HTMLDivElement>(null);
  const user = session!.user!;
  useEffect(() => {
    if (!open) return;
    const close = (e: MouseEvent) => !ref.current?.contains(e.target as Node) && setOpen(false);
    const esc = (e: KeyboardEvent) => e.key === "Escape" && setOpen(false);
    document.addEventListener("mousedown", close);
    document.addEventListener("keydown", esc);
    return () => {
      document.removeEventListener("mousedown", close);
      document.removeEventListener("keydown", esc);
    };
  }, [open]);
  const name = user.givenName || user.name || user.email;
  return (
    <div className="usermenu" ref={ref}>
      <button type="button" className="userbtn" aria-haspopup="menu" aria-expanded={open} onClick={() => setOpen((o) => !o)}>
        <Avatar name={user.name || user.email} image={user.image} size={28} />
        <span className="uname">{name}</span>
        <span aria-hidden="true" className="caret">▾</span>
      </button>
      {open ? (
        <div className="menu" role="menu">
          <div className="menu-head">
            <b>{user.name || name}</b>
            <span className="muted">{user.email}</span>
          </div>
          <NavLink to="/account" role="menuitem" onClick={() => setOpen(false)}>Your data</NavLink>
          <NavLink to="/privacy" role="menuitem" onClick={() => setOpen(false)}>Privacy Policy</NavLink>
          <NavLink to="/terms" role="menuitem" onClick={() => setOpen(false)}>Terms of Service</NavLink>
          <button
            type="button"
            role="menuitem"
            onClick={async () => {
              setOpen(false);
              await api.logout().catch(() => {});
              await refresh();
              navigate("/");
            }}
          >
            Sign out
          </button>
        </div>
      ) : null}
    </div>
  );
}

function Header() {
  const { session } = useSession();
  const location = useLocation();
  const user = session?.user;
  const signedIn = Boolean(user?.termsAccepted);
  return (
    <header className="top">
      <Link to={user ? "/dashboard" : "/"} className="brand" style={{ color: "inherit", textDecoration: "none" }}>
        <Logo /> AceMate
      </Link>
      {signedIn ? (
        <nav className="mainnav" aria-label="Main">
          {NAV.map((n) =>
            n.children ? (
              <NavGroup key={n.to} item={n} />
            ) : (
              <NavLink key={n.to} to={n.to} end className={() => `navpill ${isActive(n, location.pathname) ? "active" : ""}`}>
                {n.label}
              </NavLink>
            ),
          )}
        </nav>
      ) : (
        <div style={{ flex: 1 }} />
      )}
      {signedIn ? <Processing /> : null}
      {user ? <UserMenu /> : <Link to="/signin" className="btn" style={{ padding: "7px 14px" }}>Sign in</Link>}
    </header>
  );
}

/** Shows documents being processed in the background; opens the upload window. */
function Processing() {
  const { busy, setOpen } = useDocs();
  if (!busy) return null;
  return (
    <button type="button" className="processing" onClick={() => setOpen(true)} title="Show progress">
      <span className="spinner" aria-hidden="true" /> {busy} processing
    </button>
  );
}

// Signed-in pages. Until the current Terms and Privacy Policy are accepted, they show the
// consent screen instead (the API enforces the same rule).
function Private({ children }: { children: React.ReactNode }) {
  const { session } = useSession();
  if (!session?.user) return <Navigate to="/signin" replace />;
  if (!session.user.termsAccepted) return <Consent />;
  return <>{children}</>;
}

function Footer() {
  const location = useLocation();
  const { session } = useSession();
  if (location.pathname === "/dashboard") return null; // the profile map fills the screen
  return (
    <footer className="foot">
      <Link to="/terms">Terms of Service</Link>
      <Link to="/privacy">Privacy Policy</Link>
      {session?.version ? <span className="muted">AceMate v{session.version}</span> : null}
    </footer>
  );
}

export function App() {
  const location = useLocation();
  return (
    <DocsProvider>
      <Header />
      <main className={location.pathname === "/dashboard" ? "full" : ""}>
        <Routes>
          <Route path="/" element={<Landing />} />
          <Route path="/signin" element={<SignIn />} />
          <Route path="/terms" element={<Terms />} />
          <Route path="/privacy" element={<Privacy />} />
          <Route path="/dashboard" element={<Private><Dashboard /></Private>} />
          <Route path="/jobs" element={<Private><Jobs /></Private>} />
          <Route path="/jobs/new" element={<Private><NewJob /></Private>} />
          <Route path="/jobs/:id" element={<Private><JobPage /></Private>} />
          <Route path="/account" element={<Private><Account /></Private>} />
          <Route path="*" element={<div className="narrow card" style={{ marginTop: 40 }}>Page not found. <Link to="/">Home</Link></div>} />
        </Routes>
      </main>
      <Footer />
      <AddDocumentModal />
    </DocsProvider>
  );
}
