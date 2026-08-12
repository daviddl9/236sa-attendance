import Sidebar from "@/components/sidebar";
import DashboardTopNav from "./navbar";
import FirstTimeSignInModal from "./first-time-signin-modal";
import PasswordChangeGuard from "./password-change-guard";

export default function DashboardLayout({ children }: { children: React.ReactNode }) {
  return (
    <PasswordChangeGuard>
      <div className="flex h-screen overflow-hidden w-full">
        <Sidebar />
        <main className="flex-1 overflow-y-auto">
          <DashboardTopNav>{children}</DashboardTopNav>
        </main>
        <FirstTimeSignInModal />
      </div>
    </PasswordChangeGuard>
  );
}
