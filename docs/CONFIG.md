# Configuration format guidelines

These are the rules for every file itos-template reads or writes as configuration: a template's
manifest, `itos-template.yaml`, and the record a made project keeps, `.itos-template.yaml`
(`docs/manifest.md` is their format). Each rule names its source. No one guide covers configuration
as CLIG covers command lines, so these gather what several hold, with the person's principles
(decision 22). When itos-template does not follow a rule yet, the rule says so and names the item
that will.

Use these rules when you add or change a key, a value or a file. A decision record in
`docs/decisions/` can change a rule; change this document in the same commit. `docs/CLI.md` holds
the rules for flags, environment variables and where configuration is read from.

## Sources

| Key      | Source                                                                                                                                       |
| -------- | -------------------------------------------------------------------------------------------------------------------------------------------- |
| K8S      | Kubernetes API Conventions, <https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md>      |
| AIP      | Google API Improvement Proposals, <https://google.aip.dev>; the number after the key names the proposal, for example AIP `126`                |
| CLIG     | Command Line Interface Guidelines, <https://clig.dev>, `#configuration`                                                                      |
| JSON     | RFC 8259, The JavaScript Object Notation (JSON) Data Interchange Format, <https://www.rfc-editor.org/rfc/rfc8259>                             |
| YAML     | YAML Ain't Markup Language, version 1.2.2, <https://yaml.org/spec/1.2.2/>                                                                    |
| SCHEMA   | JSON Schema, <https://json-schema.org>                                                                                                       |
| PERSON   | The person's principles for this repository, decision 22                                                                                     |

## The principles

- **Convention over configuration, made visible.** A key may have a default, so a file says only
  what differs; every default is written down and can be printed, so nothing is implicit to a
  reader (PERSON). In code, convention pays off on its own; in a configuration file, a reader
  cannot learn it by reading the file, so the file's tool shows it.
- **Explicit is better than implicit.** A value that changes what a tool does is written in the
  file or shown as a default, never inferred from something else (PERSON).
- **JSON is the data model, YAML its syntax.** A file is plain JSON data: objects, arrays,
  strings, numbers, booleans and null (JSON). People write it as YAML, which reads JSON as well
  (YAML), so a tool may write JSON into the same `.yaml` file and it reads the same (PERSON).
- **No language of our own.** A format that wants loops, functions or shared fragments is a job
  for a configuration language that writes JSON (CUE, Pkl, Jsonnet, Dhall, or one of the person's
  own), on the author's side of the file; itos-template reads only its output (PERSON).

## Rules

### Reading a file

1. Read a file as YAML into JSON's data model, and refuse what JSON cannot say: custom tags
   (`!!python/object`, `!foo`), several documents in one file, anchors, aliases and merge keys
   (`&x`, `*x`, `<<`), keys that are not strings, and a key given twice. A tag can make a reader
   build any object (the class behind CVE-2022-1471); an alias can expand without bound (a "billion
   laughs", CVE-2019-11253); and a template's manifest comes from anything git clone takes
   (decision 9), so a file is untrusted. Repetition anchors would remove is met by the format's
   structure first (a root's checks run on every render), then by a configuration language on the
   author's side (PERSON; YAML; JSON). The rule refuses what JSON's data model cannot say, not every
   YAML spelling, so what never reaches the data is taken (quotes, comments, block or flow style),
   and so is the non-specific tag `!`, the one tag allowed: it marks a plain value as no other
   tag's, building no object and expanding nothing, and is read as if it were not there.
2. Refuse a key the format does not list, so a misspelt key never passes for an option (K8S;
   PERSON).
3. Name every problem at once, each by the path of its key from the top (`stacks[1].checks[0]`),
   so an author fixes them in one go (AIP `193`). Not yet the paths: the idea
   config-key-paths.
4. Check every value before using any: a reference to another part of the file (a feature's
   stack, its needs) names something the file holds; a pattern is valid; a default is an answer its
   question takes.

### Versions and change

5. Begin every file with `version`, an integer. A tool refuses a file whose version it does not
   know, so a file written for a later tool is never misread by an earlier one (K8S).
6. A new key comes with a new version that names it. A version keeps reading every file of the
   versions before it, as they were written (K8S; AIP `180`).
7. Removing a key, renaming it or changing what a value means is a breaking change: a major
   release, and an `Upgrading:` footer saying what an author changes (AIP `180`; `docs/CLI.md`,
   "Changing the interface").

### Shape

8. Name keys in snake case, one style in every file (AIP `140`).
9. Write a list of things with names as a list of objects, each with its `name`, never as an
   object keyed by name: order is kept, and an object's members stay the same shape (K8S, "Lists of
   named subobjects preferred over maps").
10. Prefer a string from a known set over a boolean for anything that may grow a third value, and a
    list over a single value for anything that may hold more than one (`scans: [credentials]`) (K8S,
    "Primitive types"; AIP `126`). `case_forms` is a boolean that would change format if forms
    became selectable.
11. Write a path relative to the file's top, with `/`, on every system.
12. Keep a value whole: a list of words, not a string a tool splits (a check is `[go, test,
    ./...]`, decision 15).
13. Hold no secret. A credential belongs in the environment or a secret store, never in a file a
    repository keeps (PLAN.md's principles).

### Defaults and documentation

14. Write every key, its type, whether it is required and its default where people read the
    format (`docs/manifest.md`), with an example.
15. Let a tool print a file as it reads it, every default filled in, so convention stays visible
    (PERSON; CLIG `#configuration`). Not yet: the idea config-print-defaults.
16. Publish a JSON Schema of each format, versioned with it, so an editor checks and completes a
    file and a tool that writes one has its contract (SCHEMA). Not yet: the idea config-schema.

### Writing a file

17. A file itos-template writes (the record) is written in the format's own order, a key per line,
    with no YAML-only feature, so what a tool writes reads back the same and a diff shows only what
    changed.
