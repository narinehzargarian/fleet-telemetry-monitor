// Typed client for the fleet API.

export interface StationStatus {
  station_id: string;
  last_seen: string;
  cpu_temp_c: number;
  signal_snr_db: number;
  disk_used_pct: number;
  online: boolean;
  open_alerts: number;
}

export interface Alert {
  id: number;
  created_at: string;
  station_id: string;
  reading_id: string;
  rule: string;
  severity: "warning" | "critical";
  message: string;
}

const BASE = import.meta.env.VITE_API_URL ?? "http://localhost:8080";

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(`${BASE}${path}`, init);
  if (!res.ok) {
    const body = await res.json().catch(() => ({}));
    throw new Error(body.error ?? `request failed (${res.status})`);
  }
  return res.json() as Promise<T>;
}

export const fetchStations = () => request<StationStatus[]>("/api/v1/stations");
export const fetchAlerts = () => request<Alert[]>("/api/v1/alerts?limit=25");
export const resolveAlert = (id: number) =>
  request<{ status: string }>(`/api/v1/alerts/${id}/resolve`, { method: "POST" });
