import { lazy, Suspense } from "react";
import { Navigate, Route, Routes } from "react-router-dom";
import { Shell } from "@/components/layout/Shell";
import { Loading } from "@/components/ui/Layout";
import { useAuth } from "@/state/auth";
import Login from "@/pages/Login";
import Sso from "@/pages/Sso";
import Profile from "@/pages/Profile";
import HostingOverview from "@/pages/cpanel/Overview";
import Domains from "@/pages/cpanel/Domains";
import FileManager from "@/pages/cpanel/FileManager";
import Databases from "@/pages/cpanel/Databases";
import Email from "@/pages/cpanel/Email";
import Dns from "@/pages/cpanel/Dns";
import Ssl from "@/pages/cpanel/Ssl";
import Cron from "@/pages/cpanel/Cron";
import Apps from "@/pages/cpanel/Apps";
import Ssh from "@/pages/cpanel/Ssh";
import Backups from "@/pages/cpanel/Backups";
import Installer from "@/pages/cpanel/Installer";
import WordPress from "@/pages/cpanel/WordPress";
import Php from "@/pages/cpanel/Php";
import Redirects from "@/pages/cpanel/Redirects";
import ErrorPages from "@/pages/cpanel/ErrorPages";
import Logs from "@/pages/cpanel/Logs";
import DiskUsage from "@/pages/cpanel/DiskUsage";
import Tickets from "@/pages/Tickets";
import ServerOverview from "@/pages/whm/ServerOverview";
import Accounts from "@/pages/whm/Accounts";
import Packages from "@/pages/whm/Packages";
import Resellers from "@/pages/whm/Resellers";
import Services from "@/pages/whm/Services";
import Firewall from "@/pages/whm/Firewall";
import Audit from "@/pages/whm/Audit";
import Settings from "@/pages/whm/Settings";

// xterm.js is large, so the terminal is loaded only when opened.
const TerminalPage = lazy(() => import("@/pages/cpanel/Terminal"));

// The same hosting pages serve both an account holder (/hosting) and an
// administrator managing a customer (/accounts/:aid).
const hostingRoutes = (
  <>
    <Route index element={<HostingOverview />} />
    <Route path="domains" element={<Domains />} />
    <Route path="files" element={<FileManager />} />
    <Route path="ssl" element={<Ssl />} />
    <Route path="apps" element={<Apps />} />
    <Route path="databases" element={<Databases />} />
    <Route path="email" element={<Email />} />
    <Route path="dns" element={<Dns />} />
    <Route path="cron" element={<Cron />} />
    <Route path="ssh" element={<Ssh />} />
    <Route path="backups" element={<Backups />} />
    <Route path="installer" element={<Installer />} />
    <Route path="wordpress" element={<WordPress />} />
    <Route path="php" element={<Php />} />
    <Route path="redirects" element={<Redirects />} />
    <Route path="error-pages" element={<ErrorPages />} />
    <Route path="logs" element={<Logs />} />
    <Route path="disk" element={<DiskUsage />} />
    <Route
      path="terminal"
      element={
        <Suspense fallback={<Loading />}>
          <TerminalPage />
        </Suspense>
      }
    />
  </>
);

export default function App() {
  const { user, loading } = useAuth();

  return (
    <Routes>
      <Route path="/sso" element={<Sso />} />
      {loading ? (
        <Route path="*" element={<Loading />} />
      ) : !user ? (
        <Route path="*" element={<Login />} />
      ) : (
        <Route element={<Shell />}>
          {user.role === "user" ? (
            <>
              <Route path="/" element={<Navigate to="/hosting" replace />} />
              <Route path="/hosting">{hostingRoutes}</Route>
            </>
          ) : (
            <>
              <Route path="/" element={<ServerOverview />} />
              <Route path="/accounts" element={<Accounts />} />
              <Route path="/accounts/:aid">{hostingRoutes}</Route>
              <Route path="/packages" element={<Packages />} />
              {(user.role === "root" || user.is_master) && <Route path="/resellers" element={<Resellers />} />}
              {user.role === "root" && (
                <>
                  <Route path="/services" element={<Services />} />
                  <Route path="/firewall" element={<Firewall />} />
                  <Route path="/audit" element={<Audit />} />
                  <Route path="/settings" element={<Settings />} />
                </>
              )}
            </>
          )}
          <Route path="/profile" element={<Profile />} />
          <Route path="/tickets" element={<Tickets />} />
          <Route path="*" element={<Navigate to="/" replace />} />
        </Route>
      )}
    </Routes>
  );
}
