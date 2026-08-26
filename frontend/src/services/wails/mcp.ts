/**
 * Wails 服务门面 - MCP 域
 * ============ MCP 服务器管理 ============
 * 新架构：豆芽不直接管理 MCP 子进程，而是生成 mcp_servers.json 交给 llama-server 加载。
 * 修改配置后需重启 llama-server 才能生效（无热重载）。
 * （从原 wails.ts 迁移,方法体逐字搬移,逻辑零变化）
 */
import {
  GetMCPServers,
  SaveMCPServers,
  TestMCPConnection,
  GetMCPStatus,
  ListMCPTools,
  RefreshMcpTools
} from '../../../wailsjs/go/main/App'
import type { MCPConnectResult, MCPServerConfig, MCPServerStatus, MCPToolInfo } from './types'

export const mcpMethods = {
  getMCPServers: async (): Promise<MCPServerConfig[]> => {
    return (await GetMCPServers()) as MCPServerConfig[]
  },
  saveMCPServers: async (servers: MCPServerConfig[]): Promise<void> => {
    await SaveMCPServers(servers as Parameters<typeof SaveMCPServers>[0])
  },
  testMCPConnection: async (server: MCPServerConfig): Promise<MCPConnectResult> => {
    return (await TestMCPConnection(
      server as Parameters<typeof TestMCPConnection>[0]
    )) as MCPConnectResult
  },
  getMCPStatus: async (): Promise<Record<string, MCPServerStatus>> => {
    return (await GetMCPStatus()) as Record<string, MCPServerStatus>
  },
  listMCPTools: async (): Promise<MCPToolInfo[]> => {
    return (await ListMCPTools()) as MCPToolInfo[]
  },
  refreshMcpTools: async (): Promise<void> => {
    await RefreshMcpTools()
  }
} as const
