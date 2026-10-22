# Upstream source

This directory contains only the `inno` core library source from
<https://github.com/russellbanks/inno/tree/6bd3b6d20aef9b655e6eaa2b3c9c6bd0331c902f/core>.
The upstream version is 0.7.0. Examples, CLI code, external test fixtures, and other
workspace members are excluded. The included source and notices occupy about
550 KiB.

`UPSTREAM-SHA256SUMS` records each original source file's hash. All source files
are unchanged except the two files in `PATCH.diff`. The patch stores the parsed
file condition and exposes a getter. Upstream read and discarded this value.
Conditions remain data. This library does not execute them.

The local Cargo manifest replaces inherited workspace metadata with its exact
upstream values and removes external fixture test dependencies. The MIT and
Apache-2.0 license files are copied unchanged from the pinned source repository.

