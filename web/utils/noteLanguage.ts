// The markdown dialect the notes editor parses.
//
// Deliberately GFM (`markdownLanguage`), NOT lang-markdown's default, which is
// plain CommonMark: tables, strikethrough and task lists are only ever *parsed*
// under GFM, and a construct that never reaches the syntax tree cannot be styled
// by the live preview no matter what the decoration layer does. Keeping the
// dialect in one function is what stops the editor and its tests from parsing
// two different languages and disagreeing about what exists.

import { LanguageDescription } from '@codemirror/language'
import { markdown, markdownLanguage } from '@codemirror/lang-markdown'
import { languages } from '@codemirror/language-data'

// Resolve a fenced block's info string to the grammar that should parse it.
// `@codemirror/language-data` owns both the grammars and their alias lists
// (`ts`, `js`, `sh`, `py`, ...), and holds each grammar behind a dynamic
// `load()`: nothing is imported or parsed until the first fence of that
// language is actually parsed, so a note that is all prose never pays for a
// code grammar and the notes bundle stays grammar-free beyond markdown.
// Unknown, empty or unsupported info strings resolve to null, which leaves the
// block as plain code exactly as before.
export function noteCodeLanguage(info: string): LanguageDescription | null {
  return LanguageDescription.matchLanguageName(languages, info)
}

export function noteMarkdown() {
  const support = markdown({ base: markdownLanguage, codeLanguages: noteCodeLanguage })

  return [
    support,
    // Which characters `closeBrackets` treats as pairs. The default list carries
    // `'` and `"`, and auto-closing those in prose produces `don''t`; quotes and
    // backticks are handled as selection-wrappers instead (see ./noteEditing).
    support.language.data.of({
      closeBrackets: { brackets: ['(', '[', '{'] },
    }),
  ]
}
