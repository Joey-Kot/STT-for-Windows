// Copyright (C) 2026 Joey Kot <joey.kot.x@gmail.com>
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed WITHOUT ANY WARRANTY; without even the
// implied warranty of MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.
// See <https://www.gnu.org/licenses/> for more details.

const fs = require("fs");
const path = require("path");

const root = __dirname;
const src = path.join(root, "src");
const dist = path.join(root, "dist");
const styles = path.join(root, "styles");
const generated = path.join(root, "wailsjs");

function copyDir(from, to) {
  if (!fs.existsSync(from)) {
    return;
  }
  fs.mkdirSync(to, { recursive: true });
  for (const entry of fs.readdirSync(from, { withFileTypes: true })) {
    const source = path.join(from, entry.name);
    const target = path.join(to, entry.name);
    if (entry.isDirectory()) {
      copyDir(source, target);
    } else {
      fs.copyFileSync(source, target);
    }
  }
}

fs.rmSync(dist, { recursive: true, force: true });
fs.mkdirSync(dist, { recursive: true });
copyDir(src, dist);
copyDir(styles, path.join(dist, "styles"));
copyDir(generated, path.join(dist, "wailsjs"));
