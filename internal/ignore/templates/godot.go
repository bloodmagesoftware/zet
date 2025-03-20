package ignore_templates

const Godot = `# Godot 4+ specific ignores
.godot/

# Godot-specific ignores
.import/
export.cfg
export_credentials.cfg

# Imported translations (automatically generated from CSV files)
*.translation

# Mono-specific ignores
.mono/
data_*/
mono_crash.*.json

# OS
.DS_Store

# Git
.git/
.gitattributes
.gitignore`
