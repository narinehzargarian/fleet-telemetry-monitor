import { useCallback, useEffect, useState } from "react";
import { fetchAlerts, fetchStations, resolveAlert, type Alert, type StationStatus } from "./api";
import "./App.css";

const REFRESH_MS = 3000;
// A station that hasn't reported in this long is treated as stale even if its last reading said "online".
const STALE_AFTER_MS = 30_000;

function stationHealth(s: StationStatus, now: number): "offline" | "stale" | "alerting" | "healthy" {
  if (!s.online) return "offline";
  if (now - new Date(s.last_seen).getTime() > STALE_AFTER_MS) return "stale";
  if (s.open_alerts > 0) return "alerting";
  return "healthy";
}

export default function App() {
  const [stations, setStations] = useState<StationStatus[]>([]);
  const [alerts, setAlerts] = useState<Alert[]>([]);
  const [error, setError] = useState<string | null>(null);
  // Time of the last poll; set from the polling callback (not during render) so staleness is stable per refresh.
  const [now, setNow] = useState(0);

  const load = useCallback(async () => {
    try {
      const [s, a] = await Promise.all([fetchStations(), fetchAlerts()]);
      setStations(s);
      setAlerts(a);
      setError(null);
    } catch (e) {
      setError(e instanceof Error ? e.message : "could not reach API");
    }
    setNow(Date.now());
  }, []);

  useEffect(() => {
    // Poll the API (an external system). The first poll is scheduled, not run synchronously in the effect.
    const first = setTimeout(load, 0);
    const timer = setInterval(load, REFRESH_MS);
    return () => {
      clearTimeout(first);
      clearInterval(timer);
    };
  }, [load]);

  async function onResolve(id: number) {
    try {
      await resolveAlert(id);
      setAlerts((prev) => prev.filter((a) => a.id !== id));
    } catch (e) {
      setError(e instanceof Error ? e.message : "could not resolve alert");
    }
  }

  const online = stations.filter((s) => s.online).length;

  return (
    <main className="page">
      <header>
        <h1>Fleet Telemetry Monitor</h1>
        <p className="summary">
          {online}/{stations.length} stations online · {alerts.length} open alerts
        </p>
        {error && <p className="error">⚠ {error}</p>}
      </header>

      <section>
        <h2>Ground stations</h2>
        <div className="grid">
          {stations.map((s) => {
            const health = stationHealth(s, now);
            return (
              <article key={s.station_id} className={`card ${health}`}>
                <div className="card-head">
                  <strong>{s.station_id}</strong>
                  <span className={`badge ${health}`}>{health}</span>
                </div>
                <dl>
                  <dt>CPU</dt><dd>{s.cpu_temp_c.toFixed(1)}°C</dd>
                  <dt>Signal</dt><dd>{s.signal_snr_db.toFixed(1)} dB</dd>
                  <dt>Disk</dt><dd>{s.disk_used_pct.toFixed(0)}%</dd>
                  <dt>Alerts</dt><dd>{s.open_alerts}</dd>
                </dl>
                <small>last seen {new Date(s.last_seen).toLocaleTimeString()}</small>
              </article>
            );
          })}
          {stations.length === 0 && !error && <p>Waiting for telemetry…</p>}
        </div>
      </section>

      <section>
        <h2>Open alerts</h2>
        {alerts.length === 0 ? (
          <p>No open alerts.</p>
        ) : (
          <table>
            <thead>
              <tr><th>Time</th><th>Station</th><th>Severity</th><th>Message</th><th></th></tr>
            </thead>
            <tbody>
              {alerts.map((a) => (
                <tr key={a.id}>
                  <td>{new Date(a.created_at).toLocaleTimeString()}</td>
                  <td>{a.station_id}</td>
                  <td><span className={`badge ${a.severity}`}>{a.severity}</span></td>
                  <td>{a.message}</td>
                  <td><button onClick={() => onResolve(a.id)}>Resolve</button></td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </section>
    </main>
  );
}
