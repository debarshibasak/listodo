import { useEffect, useRef } from "react"
import { EditorContent, Extension, InputRule, useEditor, type Editor } from "@tiptap/react"
import StarterKit from "@tiptap/starter-kit"
import { TaskItem, TaskList } from "@tiptap/extension-list"
import { TableKit } from "@tiptap/extension-table"
import Image from "@tiptap/extension-image"
import Placeholder from "@tiptap/extension-placeholder"
import { Markdown } from "@tiptap/markdown"
import { Plugin } from "@tiptap/pm/state"
import { Textarea } from "@/components/ui/textarea"

const MarkdownTaskItem = TaskItem.extend({
  addInputRules() {
    return [new InputRule({
      find: /^\s*(?:[-*+]\s+)?\[([ xX])\]\s$/,
      handler: ({ chain, range, match }) => {
        // The preceding "- " may already have become a bullet list.
        chain().deleteRange(range).toggleTaskList().updateAttributes("taskItem", { checked: match[1].toLowerCase() === "x" }).run()
      },
    })]
  },
})

const TaskEditing = Extension.create({
  name: "taskEditing",
  addKeyboardShortcuts() {
    return {
      Tab: () => this.editor.isActive("codeBlock") && this.editor.commands.insertContent("  "),
      "Mod-Shift-x": () => {
        if (!this.editor.isActive("taskItem")) return false
        return this.editor.commands.updateAttributes("taskItem", { checked: !this.editor.getAttributes("taskItem").checked })
      },
    }
  },
  addProseMirrorPlugins() {
    return [new Plugin({
      filterTransaction: transaction => !transaction.docChanged || !this.editor.markdown ||
        this.editor.markdown.serialize(transaction.doc.toJSON()).length <= 30000,
    })]
  },
})

export function TaskMarkdownInput({ value, onChange, disabled, source }: {
  value: string
  onChange: (value: string) => void
  disabled: boolean
  source: boolean
}) {
  const instance = useRef<Editor | null>(null)
  const currentMarkdown = useRef(value)
  const editor = useEditor({
    extensions: [
      StarterKit.configure({ link: { openOnClick: false, markdownLinks: true }, underline: false, trailingNode: false }),
      TaskList,
      MarkdownTaskItem.configure({ nested: true, HTMLAttributes: { "data-type": "taskItem" }, a11y: { checkboxLabel: node => `Complete ${node.textContent || "checklist item"}` } }),
      TableKit.configure({ table: { renderWrapper: true } }),
      Image.configure({ allowBase64: true, HTMLAttributes: { loading: "lazy", referrerpolicy: "no-referrer" } }),
      Placeholder.configure({ placeholder: "What needs to get done?", showOnlyCurrent: false }),
      Markdown.configure({ markedOptions: { gfm: true, breaks: true }, indentation: { style: "space", size: 2 } }),
      TaskEditing,
    ],
    content: value,
    contentType: "markdown",
    editable: !disabled,
    autofocus: "start",
    editorProps: {
      attributes: { class: "task-rich-editor", role: "textbox", "aria-label": "Task", "aria-multiline": "true", spellcheck: "true", tabindex: "0" },
      handlePaste: (_view, event) => {
        const text = event.clipboardData?.getData("text/plain")
        if (!text || !instance.current) return false
        event.preventDefault()
        instance.current.commands.insertContent(text, { contentType: "markdown" })
        return true
      },
      clipboardTextSerializer: slice => instance.current?.markdown?.serialize({ type: "doc", content: slice.content.toJSON() }) || slice.content.textBetween(0, slice.content.size, "\n"),
      handleKeyDown: (view, event) => {
        if (!event.isComposing && (event.metaKey || event.ctrlKey) && event.key === "Enter") {
          event.preventDefault()
          event.stopPropagation()
          view.dom.closest("form")?.requestSubmit()
          return true
        }
        return false
      },
      handleClick: (_view, _position, event) => {
        const link = event.target instanceof Element ? event.target.closest<HTMLAnchorElement>("a[href]") : null
        if (!link || !(event.metaKey || event.ctrlKey) || !/^(https?:|mailto:)/i.test(link.href)) return false
        // Use an actual link activation so WKWebView can open the system browser.
        event.preventDefault()
        const anchor = document.createElement("a")
        anchor.href = link.href
        anchor.target = navigator.userAgent.includes("Listodo/") ? "_self" : "_blank"
        anchor.rel = "noopener noreferrer"
        document.body.append(anchor)
        anchor.click()
        anchor.remove()
        return true
      },
    },
    onCreate: ({ editor }) => { instance.current = editor },
    onUpdate: ({ editor }) => {
      const markdown = editor.getMarkdown()
      currentMarkdown.current = markdown
      onChange(markdown)
    },
  })

  useEffect(() => {
    instance.current = editor
    return () => { instance.current = null }
  }, [editor])

  useEffect(() => {
    if (!editor || value === currentMarkdown.current) return
    currentMarkdown.current = value
    editor.commands.setContent(value, { contentType: "markdown", emitUpdate: false })
  }, [editor, value])

  useEffect(() => { editor?.setEditable(!disabled, false) }, [editor, disabled])
  useEffect(() => { if (!source && !disabled) editor?.commands.focus() }, [editor, source, disabled])

  return <>
    <div className="task-editor-document" hidden={source} onKeyDown={event => {
      // Radix's modal focus loop must not move focus after the editor handles Tab.
      if (event.defaultPrevented) event.stopPropagation()
    }}><EditorContent className="task-editor-content" editor={editor} /></div>
    {source && <Textarea
      aria-label="Task Markdown"
      autoFocus
      required
      disabled={disabled}
      maxLength={30000}
      value={value}
      onChange={event => onChange(event.target.value)}
    />}
  </>
}
