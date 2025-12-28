import { createRoot } from "react-dom/client";
import App from "./App.tsx";
import "./index.css";
import { setupApiInterceptors } from "./lib/api-interceptors";

// Initialize API interceptors
setupApiInterceptors();

// Application bootstrap
const rootElement = document.getElementById("root");
if (rootElement) {
  createRoot(rootElement).render(<App />);
}
