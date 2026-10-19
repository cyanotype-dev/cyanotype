// Package resolve checks a spec against the catalog and turns it into a
// resolved selection that renderers can use.
//
// internal/spec checks cyanotype.yaml on its own; everything that needs the
// catalog happens here: whether selected entries exist, exclusivity inside
// radio categories, requires and conflicts, placeholders such as
// {languages}, known answer keys, and the catalog_version pin. Problems
// that would produce wrong or inconsistent files are errors; the others are
// warnings. Both point to the field and line in the spec file.
package resolve
