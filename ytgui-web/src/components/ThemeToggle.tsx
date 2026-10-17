import { useState } from "react";
import { Moon, Sun } from "./Icons";

export type Theme = "dark" | "light";
const KEY = "fetchly.theme";

function current(): Theme {
  return document.documentElement.getAttribute("data-theme") === "light" ? "light" : "dark";
}

/** Switches data-theme on <html>. public/theme-init.js applies the saved choice before first paint. */
export function ThemeToggle() {
  const [theme, setTheme] = useState<Theme>(current);
  const next: Theme = theme === "dark" ? "light" : "dark";

  const toggle = () => {
    const root = document.documentElement;
    root.setAttribute("data-theme", next);
    root.style.colorScheme = next;
    try {
      localStorage.setItem(KEY, next);
    } catch {
      /* storage blocked: the choice lasts for this page only */
    }
    setTheme(next);
  };

  return (
    <button
      type="button"
      className="navbtn"
      onClick={toggle}
      aria-label={`Switch to ${next} theme`}
      title={`Switch to ${next} theme`}
    >
      {theme === "dark" ? <Moon size={19} /> : <Sun size={19} />}
    </button>
  );
}
