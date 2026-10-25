import { useImageLanguage } from './language'
import { useEffect } from 'react'
import { initStore } from './store'
import { useStore } from './store'
import { buildSettingsFromUrlParams, clearUrlSettingParams, hasUrlSettingParams } from './lib/urlSettings'
import { mergeImportedSettings } from './lib/apiProfiles'
import { getCustomProviderConfigUrl, loadCustomProviderSettingsFromUrl } from './lib/customProviderConfigUrl'
import { useDockerApiUrlMigrationNotice } from './hooks/useDockerApiUrlMigrationNotice'
import Header from './components/Header'
import SearchBar from './components/SearchBar'
import TaskGrid from './components/TaskGrid'
import AgentWorkspace from './components/AgentWorkspace'
import EcommerceStudio from './components/EcommerceStudio'
import InputBar from './components/InputBar'
import DetailModal from './components/DetailModal'
import Lightbox from './components/Lightbox'
import SettingsModal from './components/SettingsModal'
import ConfirmDialog from './components/ConfirmDialog'
import Toast from './components/Toast'
import MaskEditorModal from './components/MaskEditorModal'
import ImageContextMenu from './components/ImageContextMenu'
import SupportPromptModal from './components/SupportPromptModal'
import { useGlobalClickSuppression } from './lib/clickSuppression'

let customProviderConfigUrlImportStarted = false

function readLaunchSettings(searchParams: URLSearchParams) {
  const raw = searchParams.get('settings')
  if (!raw) return null
  try {
    const parsed = JSON.parse(raw)
    const settings = parsed && typeof parsed === 'object' && 'settings' in parsed
      ? (parsed as { settings?: unknown }).settings
      : parsed
    return settings && typeof settings === 'object' ? settings as Record<string, unknown> : null
  } catch {
    return null
  }
}

export default function App() {
  const { language } = useImageLanguage()
  useEffect(() => { document.documentElement.lang = language }, [language])
  const setSettings = useStore((s) => s.setSettings)
  const appMode = useStore((s) => s.appMode)
  useDockerApiUrlMigrationNotice()
  useGlobalClickSuppression()

  useEffect(() => {
    const searchParams = new URLSearchParams(window.location.search)
    const requestedAppMode = searchParams.get('appMode')
    const launchSettings = readLaunchSettings(searchParams)
    const nextSettings = buildSettingsFromUrlParams(useStore.getState().settings, searchParams)

    setSettings(nextSettings)
    if (launchSettings) {
      useStore.getState().setEcommerceCapabilities({
        defaultImageModel: typeof launchSettings.defaultImageModel === 'string' ? launchSettings.defaultImageModel : undefined,
        defaultPlanModel: typeof launchSettings.defaultPlanModel === 'string' ? launchSettings.defaultPlanModel : undefined,
        supportsEcommerce: typeof launchSettings.supportsEcommerce === 'boolean' ? launchSettings.supportsEcommerce : undefined,
        disabledReason: typeof launchSettings.ecommerceDisabledReason === 'string' ? launchSettings.ecommerceDisabledReason : undefined,
      })
    }

    if (hasUrlSettingParams(searchParams)) {
      clearUrlSettingParams(searchParams)

      const nextSearch = searchParams.toString()
      const nextUrl = `${window.location.pathname}${nextSearch ? `?${nextSearch}` : ''}${window.location.hash}`
      window.history.replaceState(null, '', nextUrl)
    }

    const customProviderConfigUrl = getCustomProviderConfigUrl()
    if (customProviderConfigUrl && !customProviderConfigUrlImportStarted) {
      customProviderConfigUrlImportStarted = true
      void loadCustomProviderSettingsFromUrl(customProviderConfigUrl)
        .then((importedSettings) => {
          if (!importedSettings) return
          const state = useStore.getState()
          state.setSettings(mergeImportedSettings(state.settings, importedSettings))
        })
        .catch((error) => {
          console.warn('Failed to import custom provider config URL:', error)
        })
    }

    initStore()
    if (requestedAppMode === 'gallery' || requestedAppMode === 'agent' || requestedAppMode === 'ecommerce') {
      useStore.getState().setAppMode(requestedAppMode)
    }
  }, [setSettings])

  useEffect(() => {
    const preventPageImageDrag = (e: DragEvent) => {
      if ((e.target as HTMLElement | null)?.closest('img')) {
        e.preventDefault()
      }
    }

    document.addEventListener('dragstart', preventPageImageDrag)
    return () => document.removeEventListener('dragstart', preventPageImageDrag)
  }, [])

  return (
    <>
      <Header />
      {appMode === 'agent' ? (
        <AgentWorkspace />
      ) : appMode === 'ecommerce' ? (
        <EcommerceStudio />
      ) : (
        <main data-home-main data-drag-select-surface className="pb-48">
          <div className="safe-area-x max-w-7xl mx-auto">
            <SearchBar />
            <TaskGrid />
          </div>
        </main>
      )}
      {appMode !== 'ecommerce' && <InputBar />}
      <DetailModal />
      <Lightbox />
      <SettingsModal />
      <ConfirmDialog />
      <SupportPromptModal />
      <Toast />
      <MaskEditorModal />
      <ImageContextMenu />
    </>
  )
}
