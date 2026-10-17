import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { initToken } from "./api/client";
import { App } from "./App";
import "./styles.css";

// Read ?token=, store it, and strip it from the address bar before anything renders.
initToken();

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <App />
  </StrictMode>,
);
