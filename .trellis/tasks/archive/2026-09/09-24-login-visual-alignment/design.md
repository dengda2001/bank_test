# Design

The login template in `main.go` currently uses a local blue palette while `workspace.css` uses a green accent and lighter OKLCH surfaces. Move login-specific layout rules into a page stylesheet and reuse the workspace tokens and common control proportions. Keep a centered, compact sign-in panel with the RentOps mark and a clear two-field hierarchy; mobile remains single-column. Static assets are served before login. Preserve the existing form route and server error handling. Avoid displaying configuration variable names to the end user.
