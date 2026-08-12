// The markdown dialect the notes editor parses.
//
// Deliberately GFM (`markdownLanguage`), NOT lang-markdown's default, which is
// plain CommonMark: tables, strikethrough and task lists are only ever *parsed*
// under GFM, and a construct that never reaches the syntax tree cannot be styled
// by the live preview no matter what the decoration layer does. Keeping the
// dialect in one function is what stops the editor and its tests from parsing
// two different languages and disagreeing about what exists.

import { markdown, markdownLanguage } from '@codemirror/lang-markdown'

export function noteMarkdown() {
  const support = markdown({ base: markdownLanguage })

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
