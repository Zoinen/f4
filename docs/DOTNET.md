# .NET assemblies (`plugins/dotnet`)

[f4#1666](https://github.com/unxed/f4/issues/1666): a .NET assembly opened as a read-only tree in a file panel, in the spirit of an assembly browser.

Put the cursor on a `.dll` or `.exe` that holds .NET metadata and press **Ctrl+PgDn** (Enter is left alone). The file is only read, through `internal/dotnet`; nothing in it is loaded or run. A file without .NET metadata is not opened this way.

## The tree

* **`assembly.md`**: the report: assembly name and version, culture, the runtime version the metadata asks for, the referenced assemblies, the types by namespace and the resource names (each list is cut off after a few hundred entries, with "and N more").
* **`References/`**: one entry per referenced assembly, named `<name> <version>`. A reference whose file lies next to the assembly (the way the runtime finds a private dependency) is a folder holding *that* assembly's own tree, read when it is first opened, so you can walk from one assembly to the next. Following stops after six steps and never re-enters an assembly already on the way. A reference that is not found is a small text file with its name and version.
* **`Namespaces/<namespace>/`**: for every type a text file `<type>` and, when its methods have bodies, `<type>.il`. The types of the global namespace sit under `(global)`, and a nested type is listed beside its outer type as `Outer+Inner`.
  * `<type>` lists the type's attributes (`[Name]`), then its members in metadata order: methods with their signatures, fields with their types and names, and other members (properties, events) by name, each with its own attributes.
  * `<type>.il` holds the IL of every method with a body: instructions, branch targets, locals and named generic instantiations. ReadyToRun (precompiled) images are read through the IL they still carry.
* **`Resources/`**: the embedded resources. One stored in the file is readable with its bytes (F3); one that is not is a short note saying so.

Everything is read-only: F3 shows a listing, F5 copies it out as text, and nothing can be written into the tree.

## Limits

* The IL text of one assembly is bounded; once the budget is spent the remaining types keep their signatures, and the `.il` files say the rest was left out.
* Only the types of custom attributes are shown, not their arguments (the value blob is not decoded yet), and generic parameter constraints are not listed yet.
* Nothing is decompiled to C#.
