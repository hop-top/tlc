# Research Report: TypeScript Browser Extension Landscape 2026

**Date:** January 17, 2026
**Subject:** Analysis of Top TypeScript Browser Extension Frameworks, Structures, and Tools

## 🏆 Top Frameworks & Tools (2025/2026)
The era of manual Webpack configuration for extensions is over. The landscape is dominated by frameworks that handle the complexities of Manifest V3, HMR (Hot Module Replacement), and cross-browser bundling.

1.  **WXT (Web Extension Tools)**
    *   **Status:** The rising star and likely "winner" for 2026 new projects.
    *   **Why:** Built on **Vite**, it is significantly faster than Plasmo (Parcel-based). It is framework-agnostic (React, Vue, Svelte, Solid) and handles manifest generation automatically.
    *   **Key Feature:** "Zip to Ship" workflow—handles everything from development to publishing.

2.  **Plasmo**
    *   **Status:** The "Next.js of Extensions." Huge adoption, but showing age due to reliance on Parcel.
    *   **Why:** Strong opinionated structure (pages/popup.tsx, contents/script.ts). Excellent for React-heavy teams.
    *   **Caution:** Maintenance velocity has slowed compared to WXT; some developers report issues with newer Tailwind versions.

3.  **CRXJS**
    *   **Status:** A lower-level Vite plugin.
    *   **Why:** Best if you want a "pure" Vite setup without a framework's abstraction. You own the `manifest.json`, but it handles HMR and bundling.

## 🏗️ Popular Project Structures

### 1. The Framework Layout (WXT/Plasmo)
Modern frameworks abstract the manifest configuration into file-system routing, similar to Next.js.
*   **`entrypoints/`** (WXT) or **`contents/`** (Plasmo):
    *   `popup.html` / `popup.tsx`: The UI when clicking the extension icon.
    *   `background.ts`: The Service Worker (MV3).
    *   `content.ts`: Scripts injected into web pages.
*   **`components/`**: Shared React/Vue components.
*   **`utils/`**: Shared logic (storage, messaging).
*   **`wxt.config.ts`**: Configuration for imports, permissions, and browser targets.

### 2. The Manual Layout (Vite + CRXJS)
For teams wanting full control over the `manifest.json`.
*   **`manifest.json`**: The source of truth.
*   **`src/`**:
    *   `background/index.ts`
    *   `content/index.ts`
    *   `popup/index.tsx`
*   **`vite.config.ts`**: configured with `@crxjs/vite-plugin`.

## 🛠️ Essential Libraries & Concepts

### 1. Manifest V3 (MV3) Architecture
*   **Service Workers**: Replaced background pages. They are ephemeral (die after ~30s of inactivity).
    *   *Challenge:* You cannot store state in global variables. You must use `chrome.storage` or `IndexedDB`.
*   **Messaging**: `chrome.runtime.sendMessage` is critical for communication between Content Scripts (UI context) and Service Workers (Background context).

### 2. State & Storage
*   **`@plasmohq/storage`**: A popular hook-based wrapper for `chrome.storage` (works in WXT too).
    *   Example: `const [value] = useStorage("key")` syncs state across Popup and Options pages automatically.
*   **`webextension-polyfill`**: Mozilla's library to allow using `browser.runtime` (Promise-based) instead of `chrome.runtime` (Callback-based) for standardizing code across Firefox and Chrome.

### 3. UI & Styling
*   **Tailwind CSS**: The standard for Popup/Options UI.
*   **Shadow DOM**: Critical for Content Scripts to prevent the host page's CSS from bleeding into your extension's injected UI.
    *   Frameworks like WXT/Plasmo have built-in helpers to mount React/Vue components inside a Shadow Root.

## 📝 Strategic Recommendations
1.  **Choose WXT for New Projects**: It offers the best Developer Experience (DX) in 2026, leveraging Vite's speed and ecosystem.
2.  **Use React or Vue**: Do not use vanilla JS for Popups; the state management needs (syncing with storage) are too complex for vanilla to handle cleanly.
3.  **Abstract Messaging**: Use a typed wrapper for messaging (like `trpc-chrome` or WXT's messaging utilities) to ensure type safety between your Background and Content scripts.
4.  **Design for Ephemeral Backgrounds**: Never assume your background script stays alive. Persist everything immediately.
