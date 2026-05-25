import { ref } from 'vue'

export const useVaultContext = () => {
  const currentFile = useState('vault_current_file', () => '')
  const currentContent = useState('vault_current_content', () => '')
  return { currentFile, currentContent }
}
