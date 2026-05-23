import { useQuery } from "@tanstack/react-query";
import { Activity } from "lucide-react";

import { PageHeader } from "@/components/PageHeader";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import { api } from "@/lib/api";

export function HealthPage() {
  const { data, isLoading } = useQuery({
    queryKey: ["health"],
    queryFn: () => api.getHealth(),
    refetchInterval: 10_000,
  });

  return (
    <>
      <PageHeader title="Health" description="Liveness and database connectivity." />
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <Activity className="h-4 w-4" /> Service
          </CardTitle>
        </CardHeader>
        <CardContent className="grid gap-3 text-sm sm:grid-cols-2">
          {isLoading || !data ? (
            <>
              <Skeleton className="h-5 w-full" />
              <Skeleton className="h-5 w-full" />
              <Skeleton className="h-5 w-full" />
            </>
          ) : (
            <>
              <Row label="Service" value={data.service} />
              <Row label="Version" value={data.version} />
              <Row
                label="Status"
                value={
                  <Badge variant={data.status === "ok" ? "success" : "destructive"}>
                    {data.status}
                  </Badge>
                }
              />
              <Row
                label="DB status"
                value={
                  <Badge
                    variant={
                      data.db_status === "ok"
                        ? "success"
                        : data.db_status === "unavailable"
                          ? "warning"
                          : "destructive"
                    }
                  >
                    {data.db_status}
                  </Badge>
                }
              />
              <Row label="Timestamp" value={new Date(data.timestamp).toLocaleString()} />
            </>
          )}
        </CardContent>
      </Card>
    </>
  );
}

function Row({ label, value }: { label: string; value: React.ReactNode }) {
  return (
    <div className="flex items-center justify-between gap-3 border-b border-border/40 py-2 last:border-0">
      <span className="text-muted-foreground">{label}</span>
      <span className="font-medium">{value}</span>
    </div>
  );
}
