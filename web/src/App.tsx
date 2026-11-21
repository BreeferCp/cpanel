import { lazy, Suspense } from "react";
import { Navigate, Route, Routes } from "react-router-dom";
import { CpanelShell } from "@/components/layout/CpanelShell";
import { WhmShell } from "@/components/layout/WhmShell";
import { Loading } from "@/components/ui/Layout";
import { useAuth } from "@/state/auth";
import { usePanel } from "@/state/panel";
import Login from "@/pages/Login";
import Sso from "@/pages/Sso";
import Profile from "@/pages/Profile";
import Tickets from "@/pages/Tickets";
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
import Ftp from "@/pages/cpanel/Ftp";
import Git from "@/pages/cpanel/Git";
import Autoresponders from "@/pages/cpanel/Autoresponders";
import SpamFilters from "@/pages/cpanel/SpamFilters";
import RemoteMysql from "@/pages/cpanel/RemoteMysql";
import { DirectoryPrivacy, Hotlink, IpBlocker } from "@/pages/cpanel/WebProtection";
import ServerOverview from "@/pages/whm/ServerOverview";
import Accounts from "@/pages/whm/Accounts";
import Packages from "@/pages/whm/Packages";
import Resellers from "@/pages/whm/Resellers";
import Services from "@/pages/whm/Services";
import Firewall from "@/pages/whm/Firewall";
import Audit from "@/pages/whm/Audit";
import Settings from "@/pages/whm/Settings";
import FeatureManager from "@/pages/whm/FeatureManager";
import Updates from "@/pages/whm/Updates";
import { MailQueue, Processes, ServerInfo } from "@/pages/whm/ServerTools";
import { HostAccess, SecurityPolicy } from "@/pages/whm/SecurityCenter";
import { BackupConfiguration, Broadcast, DnsZones, QuotaReport } from "@/pages/whm/Operations";

// xterm.js is large, so the terminal is loaded only when opened.
const TerminalPage = lazy(() => import("@/pages/cpanel/Terminal"));

// The same hosting pages serve both an account holder in cPanel (/hosting)
// and an administrator managing a customer from WHM (/accounts/:aid).
const hostingRoutes = (
  <>
    <Route index element={<HostingOverview />} />
    <Route path="domains" element={<Domains />} />
    <Route path="files" element={<FileManager />} />
    <Route path="ssl" element={<Ssl />} />
    <Route path="apps" element={<Apps />} />
    <Route path="databases" element={<Databases />} />
    <Route path="remote-mysql" element={<RemoteMysql />} />
    <Route path="email" element={<Email />} />
    <Route path="autoresponders" element={<Autoresponders />} />
    <Route path="spam" element={<SpamFilters />} />
    <Route path="dns" element={<Dns />} />
    <Route path="cron" element={<Cron />} />
    <Route path="ssh" element={<Ssh />} />
    <Route path="ftp" element={<Ftp />} />
    <Route path="git" element={<Git />} />
    <Route path="backups" element={<Backups />} />
    <Route path="installer" element={<Installer />} />
    <Route path="wordpress" element={<WordPress />} />
    <Route path="php" element={<Php />} />
    <Route path="redirects" element={<Redirects />} />
    <Route path="error-pages" element={<ErrorPages />} />
    <Route path="directory-privacy" element={<DirectoryPrivacy />} />
    <Route path="ip-blocker" element={<IpBlocker />} />
    <Route path="hotlink" element={<Hotlink />} />
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

function CpanelRoutes() {
  return (
    <Route element={<CpanelShell />}>
      <Route path="/" element={<Navigate to="/hosting" replace />} />
      <Route path="/hosting">{hostingRoutes}</Route>
      <Route path="/profile" element={<Profile />} />
      <Route path="/tickets" element={<Tickets />} />
      <Route path="*" element={<Navigate to="/hosting" replace />} />
    </Route>
  );
}

function WhmRoutes({ root, master }: { root: boolean; master: boolean }) {
  return (
    <Route element={<WhmShell />}>
      <Route path="/" element={<ServerOverview />} />
      <Route path="/accounts" element={<Accounts />} />
      <Route path="/accounts/:aid">{hostingRoutes}</Route>
      <Route path="/packages" element={<Packages />} />
      <Route path="/features" element={<FeatureManager />} />
      <Route path="/quota-report" element={<QuotaReport />} />
      <Route path="/dns-zones" element={<DnsZones />} />
      <Route path="/broadcast" element={<Broadcast />} />
      {(root || master) && <Route path="/resellers" element={<Resellers />} />}
      {root && (
        <>
          <Route path="/server-info" element={<ServerInfo />} />
          <Route path="/services" element={<Services />} />
          <Route path="/processes" element={<Processes />} />
          <Route path="/updates" element={<Updates />} />
          <Route path="/mail-queue" element={<MailQueue />} />
          <Route path="/security" element={<SecurityPolicy />} />
          <Route path="/host-access" element={<HostAccess />} />
          <Route path="/firewall" element={<Firewall />} />
          <Route path="/audit" element={<Audit />} />
          <Route path="/backup-config" element={<BackupConfiguration />} />
          <Route path="/settings" element={<Settings />} />
        </>
      )}
      <Route path="/profile" element={<Profile />} />
      <Route path="/tickets" element={<Tickets />} />
      <Route path="*" element={<Navigate to="/" replace />} />
    </Route>
  );
}

export default function App() {
  const { user, loading } = useAuth();
  const panel = usePanel();
  // On a combined listener (cpanel_listen empty) the layout follows the role.
  const customer = panel.panel === "cpanel" || (panel.panel === "both" && user?.role === "user");

  return (
    <Routes>
      <Route path="/sso" element={<Sso />} />
      {loading ? (
        <Route path="*" element={<Loading />} />
      ) : !user ? (
        <Route path="*" element={<Login />} />
      ) : customer ? (
        CpanelRoutes()
      ) : (
        WhmRoutes({ root: user.role === "root", master: !!user.is_master })
      )}
    </Routes>
  );
}
