<script lang="ts">
  import { onMount } from 'svelte'
  import { EditorView, type ViewUpdate } from '@codemirror/view'
  import { EditorState } from '@codemirror/state'
  import { basicSetup } from 'codemirror'
  import { yaml } from '@codemirror/lang-yaml'
  import { tokyoNight } from '@uiw/codemirror-theme-tokyo-night'
  import { yamlLinter } from './yaml-linter'

  let {
    value = '',
    onchange,
  }: {
    value?: string
    onchange?: (value: string) => void
  } = $props()

  let editorEl: HTMLDivElement
  let view: EditorView
  let isDebounceEdit = false

  function createEditor() {
    const startState = EditorState.create({
      doc: value,
      extensions: [
        basicSetup,
        yaml(),
        tokyoNight,
        yamlLinter,
        EditorView.updateListener.of((update: ViewUpdate) => {
          if (update.docChanged && !isDebounceEdit) {
            onchange?.(update.state.doc.toString())
          }
        }),
        EditorView.theme({
          '&': {
            height: '100%',
            fontSize: '12px',
            backgroundColor: '#1a1b26',
          },
          '.cm-scroller': {
            overflow: 'auto',
            fontFamily: 'var(--font-mono)',
            lineHeight: '1.7',
          },
          '.cm-content': {
            caretColor: '#c0caf5',
          },
          '.cm-gutters': {
            backgroundColor: '#1a1b26',
            color: '#565f89',
            borderRight: '1px solid #24283b',
          },
          '&.cm-focused': {
            outline: 'none',
          },
        }),
      ],
    })

    view = new EditorView({
      state: startState,
      parent: editorEl,
    })
  }

  export function scrollToLine(line: number): void {
    if (!view) return
    const doc = view.state.doc
    if (line < 1 || line > doc.lines) return
    const lineStart = doc.line(line)
    view.dispatch({
      selection: { anchor: lineStart.from, head: lineStart.from },
      scrollIntoView: true,
    })
  }

  export function setValue(newValue: string): void {
    if (!view) return
    const current = view.state.doc.toString()
    if (current === newValue) return
    view.dispatch({
      changes: { from: 0, to: current.length, insert: newValue },
    })
  }

  $effect(() => {
    if (isDebounceEdit) return  // Defensive double-lock: skip if triggered by own onchange
    const newVal = value
    if (newVal !== undefined && view) {
      const current = view.state.doc.toString()
      if (current !== newVal) {
        isDebounceEdit = true
        view.dispatch({
          changes: { from: 0, to: current.length, insert: newVal }
        })
        isDebounceEdit = false
      }
    }
  })

  onMount(() => {
    createEditor()
    return () => {
      view?.destroy()
    }
  })
</script>

<div bind:this={editorEl} class="codemirror-editor" role="textbox" aria-label="YAML editor"></div>

<style>
  .codemirror-editor {
    height: 100%;
    width: 100%;
    border: 1px solid #24283b;
    border-radius: 0 0 var(--radius-md, 6px) var(--radius-md, 6px);
    overflow: hidden;
    background: #1a1b26;
  }
</style>
