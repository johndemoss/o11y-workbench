<script lang="ts">
  import { onMount } from 'svelte'
  import { EditorView, type ViewUpdate } from '@codemirror/view'
  import { EditorState } from '@codemirror/state'
  import { basicSetup } from 'codemirror'
  import { yaml } from '@codemirror/lang-yaml'
  import { oneDark } from '@codemirror/theme-one-dark'
  import { yamlLinter, lintGutter } from './yaml-linter'

  let {
    value = '',
    onchange,
  }: {
    value?: string
    onchange?: (value: string) => void
  } = $props()

  let editorEl: HTMLDivElement
  let view: EditorView

  function createEditor() {
    const startState = EditorState.create({
      doc: value,
      extensions: [
        basicSetup,
        yaml(),
        oneDark,
        yamlLinter,
        lintGutter(),
        EditorView.updateListener.of((update: ViewUpdate) => {
          if (update.docChanged) {
            onchange?.(update.state.doc.toString())
          }
        }),
        EditorView.theme({
          '&': { height: '100%' },
          '.cm-scroller': { overflow: 'auto' },
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
    const newVal = value
    if (newVal !== undefined && view) {
      const current = view.state.doc.toString()
      if (current !== newVal) {
        view.dispatch({
          changes: { from: 0, to: current.length, insert: newVal }
        })
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
    border: 1px solid var(--color-border, #334155);
    border-radius: var(--radius-md, 6px);
    overflow: hidden;
  }
</style>
