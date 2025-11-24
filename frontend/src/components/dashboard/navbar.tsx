import { Button } from "@/components/ui/button";
import { Dialog, DialogClose } from "@/components/ui/dialog";
import { Separator } from "@/components/ui/separator";
import {
  SheetContent,
  SheetHeader,
  SheetTitle,
  SheetTrigger,
} from "@/components/ui/sheet";
import UserProfile from "./user-profile";
import { HomeIcon, Menu, Settings, Users, Calendar, ScanLine, BarChart3 } from "lucide-react";
import { Link, useNavigate } from "@tanstack/react-router";

export default function DashboardTopNav({ children }: { children: React.ReactNode }) {
  const navigate = useNavigate();
  return (
    <div className="flex flex-col h-full overflow-hidden">
      <header className="sticky top-0 z-10 flex h-14 lg:h-[52px] items-center gap-4 border-b bg-background px-3 flex-shrink-0">
        <Dialog>
          <SheetTrigger className="min-[1024px]:hidden p-2 transition" asChild>
            <Button variant="ghost" size="icon">
              <Menu className="h-5 w-5" />
              <span className="sr-only">Toggle menu</span>
            </Button>
          </SheetTrigger>
          <SheetContent side="left">
            <SheetHeader>
              <Link to="/">
                <SheetTitle>236SA Attendance</SheetTitle>
              </Link>
            </SheetHeader>
            <div className="flex flex-col space-y-3 mt-[1rem]">
              <DialogClose asChild>
                <Link to="/dashboard">
                  <Button variant="outline" className="w-full">
                    <HomeIcon className="mr-2 h-4 w-4" />
                    Overview
                  </Button>
                </Link>
              </DialogClose>
              <Separator className="my-3" />
              <DialogClose asChild>
                <Button
                  variant="outline"
                  className="w-full"
                  onClick={() => navigate({ to: '/dashboard/users' as any })}
                >
                  <Users className="mr-2 h-4 w-4" />
                  Users
                </Button>
              </DialogClose>
              <DialogClose asChild>
                <Button
                  variant="outline"
                  className="w-full"
                  onClick={() => navigate({ to: '/dashboard/sessions' as any })}
                >
                  <Calendar className="mr-2 h-4 w-4" />
                  Sessions
                </Button>
              </DialogClose>
              <DialogClose asChild>
                <Button
                  variant="outline"
                  className="w-full"
                  onClick={() => navigate({ to: '/dashboard/attendance/scan' as any })}
                >
                  <ScanLine className="mr-2 h-4 w-4" />
                  Scan QR
                </Button>
              </DialogClose>
              <DialogClose asChild>
                <Button
                  variant="outline"
                  className="w-full"
                  onClick={() => navigate({ to: '/dashboard/reports' as any })}
                >
                  <BarChart3 className="mr-2 h-4 w-4" />
                  Reports
                </Button>
              </DialogClose>
              <Separator className="my-3" />
              <DialogClose asChild>
                <Link to="/dashboard/settings">
                  <Button variant="outline" className="w-full">
                    <Settings className="mr-2 h-4 w-4" />
                    Settings
                  </Button>
                </Link>
              </DialogClose>
            </div>
          </SheetContent>
        </Dialog>
        <div className="flex justify-center items-center gap-2 ml-auto">
          <UserProfile mini={true} />
        </div>
      </header>
      <div className="flex-1 overflow-y-auto">
        {children}
      </div>
    </div>
  );
}

