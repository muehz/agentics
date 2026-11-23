# README and file index reference

Full detail for README generation and file-index maintenance in the `docs` skill.

---

## README generation

Create README.md files for projects.

### README process

1. Review the entire project and workspace to understand the codebase.
2. Create a README that tells a new reader what the project does and how to use it.
3. Where appropriate, use GFM (GitHub Flavored Markdown) and GitHub admonition syntax.

### README guidelines

- Keep the README short.
- Do not overuse emojis.
- Do not include LICENSE, CONTRIBUTING, or CHANGELOG sections. Those topics have dedicated files.
- If a logo or icon exists for the project, use it in the header.
- Use this structure:
    - Project name or logo
    - Brief description
    - Key features
    - Prerequisites
    - Getting started
    - Usage
    - Configuration
    - Architecture overview, when applicable
    - Links to further documentation

---

## File index maintenance

Update markdown files with an index or table of the files in a specified folder.

### Index process

1. **Scan**: Read the target markdown file to understand its existing structure.
2. **Discover**: List all files in the specified folder that match the given pattern.
3. **Analyze**: Identify whether an existing table or index section is available to update. If none exists, create a new
   structure.
4. **Structure**: Generate an appropriate table or list format. Base it on the file types and the existing content.
5. **Update**: Replace the existing section with the file index. If no section exists, add a new section with the file
   index.
6. **Validate**: Make sure that the markdown syntax is valid and the formatting is consistent.

### File analysis

For each discovered file, extract:

- **Name**: The filename, with or without its extension, as the context requires
- **Type**: File extension and category
- **Description**: First line comment, header, or inferred purpose

### Index format options

**Simple List:**

```markdown
## Files in [folder]

- [filename.ext](path/to/filename.ext) - Description
```

**Detailed Table:**

| File                                 | Type      | Description |
| ------------------------------------ | --------- | ----------- |
| [filename.ext](path/to/filename.ext) | Extension | Description |

**Categorized Sections:** Group files by type or category, with separate sections or sub-tables.

### Section identification

Look for existing sections with these patterns:

- Headings that contain: "index", "files", "contents", "directory", "list"
- Tables with file-related columns
- Lists with file links
- HTML comments that mark file index sections

### Index requirements

- Preserve existing markdown structure, formatting, heading levels, and document flow.
- Use relative paths for file links.
- When file descriptions are available, include them.
- Sort files alphabetically by default.
- Handle special characters in filenames.
- Validate all generated markdown syntax.
