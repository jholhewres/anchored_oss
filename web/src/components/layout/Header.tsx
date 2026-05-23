import { LogOut, User } from "lucide-react";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { useAuth } from "@/lib/auth";

const scopeVariant = (scope: string) => {
  switch (scope) {
    case "admin":
      return "destructive";
    case "sync":
      return "default";
    case "readonly":
      return "secondary";
    default:
      return "outline";
  }
};

export function Header() {
  const { me, logout } = useAuth();
  if (!me) return null;
  return (
    <header className="flex h-14 items-center justify-between border-b bg-background/60 px-6 backdrop-blur">
      <div className="flex items-center gap-3">
        <p className="text-sm text-muted-foreground">
          Signed in as <span className="text-foreground">{me.display_name}</span>
        </p>
      </div>
      <div className="flex items-center gap-3">
        <Badge variant={scopeVariant(me.scope)} className="uppercase tracking-wider">
          {me.scope}
        </Badge>
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button variant="ghost" size="icon" aria-label="User menu">
              <User className="h-4 w-4" />
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end" className="w-56">
            <DropdownMenuLabel>{me.email}</DropdownMenuLabel>
            <DropdownMenuSeparator />
            <DropdownMenuItem
              className="text-destructive"
              onSelect={() => {
                logout();
                window.location.replace("/login");
              }}
            >
              <LogOut className="mr-2 h-4 w-4" />
              Sign out
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
      </div>
    </header>
  );
}
