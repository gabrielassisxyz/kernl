import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import TagInput from '../components/tags/TagInput.vue'

function mountInput(tags: string[] = []) {
  return mount(TagInput, { props: { modelValue: tags, 'onUpdate:modelValue': () => {} } })
}

async function type(w: ReturnType<typeof mountInput>, value: string, key = 'Enter') {
  const input = w.get('input')
  await input.setValue(value)
  await input.trigger('keydown', { key })
}

describe('TagInput', () => {
  it('adds a normalized tag on Enter', async () => {
    const w = mountInput()
    await type(w, '  Homelab/NAS ')
    expect(w.emitted('update:modelValue')?.at(-1)).toEqual([['homelab/nas']])
  })

  it('adds on comma too, since a comma cannot be part of a name', async () => {
    const w = mountInput()
    await type(w, 'reading', ',')
    expect(w.emitted('update:modelValue')?.at(-1)).toEqual([['reading']])
  })

  it('refuses a sys/ tag and says why', async () => {
    const w = mountInput()
    await type(w, 'sys/pending')
    expect(w.emitted('update:modelValue')).toBeUndefined()
    expect(w.text()).toContain('reserved')
  })

  it('refuses a name with an empty segment', async () => {
    const w = mountInput()
    await type(w, 'foo//bar')
    expect(w.emitted('update:modelValue')).toBeUndefined()
    expect(w.text()).toContain('empty segment')
  })

  it('ignores a tag already present, even under a different casing', async () => {
    const w = mountInput(['homelab'])
    await type(w, 'Homelab')
    expect(w.emitted('update:modelValue')).toBeUndefined()
  })

  it('removes a tag from its chip', async () => {
    const w = mountInput(['homelab', 'reading'])
    await w.get('[aria-label="Remove tag homelab"]').trigger('click')
    expect(w.emitted('update:modelValue')?.at(-1)).toEqual([['reading']])
  })

  it('pops the last chip on backspace with an empty draft', async () => {
    const w = mountInput(['homelab', 'reading'])
    await w.get('input').trigger('keydown', { key: 'Backspace' })
    expect(w.emitted('update:modelValue')?.at(-1)).toEqual([['homelab']])
  })
})
