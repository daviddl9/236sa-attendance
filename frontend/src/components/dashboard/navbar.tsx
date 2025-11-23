import { Button } from "@/components/ui/button";
import { Separator } from "@/components/ui/separator";
import UserProfile from "./user-profile";

export default function DashboardTopNav({ children }: { children: React.ReactNode }) {
  return (
    <div className="flex flex-col">
      <header className="flex h-14 lg:h-[52px] items-center gap-4 border-b px-3">
        <div className="flex justify-center items-center gap-2 ml-auto">
          <UserProfile mini={true} />
        </div>
      </header>
      {children}
    </div>
  );
}

