import { useEffect, useState } from "react";
import { api, errorMessage } from "../api/client";
import { ToolStatus } from "../components/ToolStatus";
import { useStore } from "../state/store";

// Module-level so React StrictMode's double mount doesn't POST twice.
let autoStarted = false;

export function Setup() {
  const { tools, toolProgress } = useStore();
  const [starting, setStarting] = useState(false);
  const [err, setErr] = useState<string | null>(null);
  const [tried, setTried] = useState(false);

  const start = async () => {
    setStarting(true);
    setErr(null);
    setTried(true);
    try {
      await api.setupTools();
    } catch (e) {
      setErr(errorMessage(e));
    } finally {
      setStarting(false);
    }
  };

  useEffect(() => {
    if (autoStarted) return;
    autoStarted = true;
    void start();
  }, []);

  const list = tools?.tools ?? [];
  const anyBusy = list.some((t) => t.busy);
  const failed = list.some((t) => t.required && !t.installed && !t.busy && (t.error || toolProgress[t.name]?.stage === "error"));
  const ready = !!tools?.ready;

  let heading = "Setting things up";
  let sub = "Fetchly needs a few free helper programs. This happens once and takes a minute or two.";
  if (ready && !anyBusy) {
    heading = "All set";
    sub = "Everything is ready.";
  } else if (failed || err) {
    heading = "Setup didn't finish";
    sub = "Check your internet connection, then try again.";
  }

  return (
    <div className="page page--narrow">
      <section className="card setup">
        <h1 className="setup__title">{heading}</h1>
        <p className="setup__sub">{sub}</p>

        <ul className="toollist">
          {list.map((t) => (
            <ToolStatus key={t.name} tool={t} progress={toolProgress[t.name]} variant="setup" />
          ))}
        </ul>

        {err && (
          <p className="form-error" role="alert">
            {err}
          </p>
        )}

        <div className="setup__actions">
          <button
            type="button"
            className="btn btn--primary btn--xl"
            onClick={start}
            disabled={starting || anyBusy}
          >
            {anyBusy || starting ? "Working…" : tried ? "Try again" : "Start"}
          </button>
        </div>
      </section>
    </div>
  );
}
