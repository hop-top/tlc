# Research Report: PHP CLI Landscape 2026

**Date:** January 17, 2026
**Subject:** Analysis of Top PHP CLI Repositories, Structures, and Frameworks

## 🏆 Top PHP CLI Repositories & Tools
The PHP CLI ecosystem is anchored by powerful developer tooling and frameworks that leverage the language's ubiquity:

1.  **Dependency Management & Quality:**
    *   **`composer/composer`**: The undisputed package manager. A masterclass in PHP CLI design.
    *   **`phpstan/phpstan`** & **`vimeo/psalm`**: Static analysis tools that run as CLIs.
    *   **`rectorphp/rector`**: Automated refactoring CLI, hugely popular for upgrading legacy codebases.
    *   **`squizlabs/php_codesniffer`** & **`friendsofphp/php-cs-fixer`**: Code style enforcers.

2.  **Application Frameworks:**
    *   **`laravel/framework` (Artisan)**: While a full web framework, the `artisan` CLI is the benchmark for developer experience (DX).
    *   **`symfony/console`**: The industry standard library powering Composer, Drupal, and Laravel's internal CLI logic.

3.  **Modern Tools:**
    *   **`laravel-zero/laravel-zero`**: A micro-framework specifically for building standalone CLI apps (not web apps).
    *   **`pestphp/pest`**: A testing framework with a beautiful CLI output, prioritizing UX.

## 🏗️ Popular Project Structures

### 1. The Standard Composer Layout
The most common structure, following PSR-4 autoloading.
*   **`bin/`**: Contains the executable entry point (e.g., `bin/my-app`).
    *   The entry point usually has `#!/usr/bin/env php` and requires the autoloader.
*   **`src/`**: Contains the logic (Commands, Services).
*   **`composer.json`**: Defines dependencies and the `bin` directive for global installation.
*   **`tests/`**: PHPUnit or Pest tests.

### 2. The Laravel Zero Layout
Mimics a simplified Laravel application.
*   **`app/`**: Contains `Commands/` and `Providers/`.
*   **`config/`**: Configuration files (app, database, cache).
*   **`builds/`**: Output directory for standalone binaries.

## 🛠️ Dominant Frameworks & Libraries

### 1. Command Frameworks
*   **`symfony/console`**: The "Cobra" of PHP. Robust, strictly typed, and enterprise-ready. It handles parsing, input, output, and helpers. Used by 90% of major PHP projects.
*   **`laravel-zero/laravel-zero`**: The "Typer" of PHP. Built on top of Symfony/Laravel, it adds a massive layer of DX (Developer Experience). Features include:
    *   Desktop notifications.
    *   Menu builders.
    *   Self-update commands.
    *   Standalone binary compilation.
*   **`mnapoli/silly`**: A micro-framework wrapper around Symfony Console for defining commands as closures (functional style).

### 2. User Interface (UI/TUI)
*   **`laravel/prompts`**: A game-changer introduced recently. It provides beautiful, usable forms (text, password, select, multi-select, search) that work in any PHP project, not just Laravel.
*   **`nunomaduro/termwind`**: Allows styling CLI output using **Tailwind CSS** classes (e.g., `<div class="bg-blue-500 text-white">`).
*   **`symfony/console` Helpers**: Classic table rendering, progress bars, and question helpers.

### 3. Compilation & Distribution (The 2025 Trend)
Historically, PHP apps required the user to have `php` installed. The new trend is **Single Binary Distribution**.
*   **`static-php-cli (spc)`**: Compiles PHP source + extensions + code into a single, dependency-free binary (Linux/macOS/Windows).
*   **`box-project/box`**: Packages PHP apps into PHARs (PHP Archives), which are single-file executables (requires PHP installed).
*   **`frankenphp`**: While mostly for web, it supports embedding PHP apps into static binaries.

## 📝 Strategic Recommendations
1.  **Use `Laravel Zero` for Apps**: If building a standalone tool for others to use, Laravel Zero offers the best starting point and compilation support.
2.  **Use `Symfony Console` for Libraries**: If adding a CLI to an existing library, stick to pure Symfony Console to minimize dependencies.
3.  **Adopt `Termwind` + `Prompts`**: For any user interaction, these libraries provide a modernized look and feel that vastly outperforms standard text output.
4.  **Target Static Binaries**: Distribute your tool as a compiled binary using `spc` or `micro` to allow users to run it without installing PHP locally.
