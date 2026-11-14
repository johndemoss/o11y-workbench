import { linter, type Diagnostic } from '@codemirror/lint'
import { load, YAMLException } from 'js-yaml'

export const yamlLinter = linter(
  (view) => {
    const text = view.state.doc.toString()
    if (!text.trim()) return []
    try {
      load(text)
      return []
    } catch (e) {
      if (e instanceof YAMLException && e.mark) {
        const lineNum = Math.min(e.mark.line + 1, view.state.doc.lines)
        const line = view.state.doc.line(lineNum)
        const from = line.from + e.mark.column
        const diagnostic: Diagnostic = {
          from,
          to: Math.min(from + 1, view.state.doc.length),
          severity: 'error',
          message: e.message,
        }
        return [diagnostic]
      }
      return []
    }
  },
  { delay: 500, autoPanel: true }
)

