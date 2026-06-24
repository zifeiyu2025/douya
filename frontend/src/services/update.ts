import { wails, type UpdateInfo } from './wails'

export type { UpdateInfo }

export async function getAppVersion(): Promise<string> {
  return wails.getAppVersion()
}

export async function checkUpdate(): Promise<UpdateInfo> {
  return wails.checkUpdate()
}

export async function performUpdate(downloadURL: string, latestVersion: string): Promise<void> {
  return wails.performUpdate(downloadURL, latestVersion)
}
