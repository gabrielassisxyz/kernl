import { useState } from '#app'

/**
 * What the open screen is currently showing, rendered in the shell footer -
 * e.g. "28 / 735 notes".
 *
 * Shared state rather than a prop because the footer belongs to the layout and
 * the count belongs to the page, and threading it through would put a
 * notes-shaped prop on every screen that has no count at all. A page sets it on
 * mount and clears it on unmount; an empty string renders nothing.
 */
export const useVaultCounter = () => useState<string>('vault-counter', () => '')
