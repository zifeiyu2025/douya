/**
 * 附件管理 composable
 * 封装图片/音频/视频/PDF/文本文件的处理逻辑
 */
import { ref, computed } from 'vue'
import type { Attachment } from '../types/chat'

const ACCEPT_MAP: Record<string, string> = {
    image: '.jpg,.jpeg,.png,.gif,.webp,.bmp,.svg',
    audio: '.wav,.mp3,.ogg,.flac,.aac,.m4a,.wma',
    text: '.txt,.md,.csv,.json,.xml,.html,.css,.js,.ts,.py,.go,.java,.c,.cpp,.h,.rs,.sh,.yaml,.yml,.toml,.ini,.cfg,.log,.sql',
    pdf: '.pdf',
    video: '.mp4,.webm,.avi,.mov,.mkv,.wmv,.flv',
}

const TYPE_LABEL_MAP: Record<string, string> = {
    image: '图片',
    audio: '音频',
    text: '文本',
    video: '视频',
    pdf: 'PDF',
}

const MAX_IMAGES = 4

export function getAcceptForType(type: string): string {
    return ACCEPT_MAP[type] || ''
}

export function getTypeLabel(type: string): string {
    return TYPE_LABEL_MAP[type] || type
}

export function useFileAttachments() {
    const attachments = ref<Attachment[]>([])
    const showMenu = ref(false)
    const pendingType = ref('image')

    const imageCount = computed(() => attachments.value.filter((a) => a.type === 'image').length)
    const canAddImage = computed(() => imageCount.value < MAX_IMAGES)

    function closeMenu() {
        showMenu.value = false
    }

    function toggleMenu() {
        showMenu.value = !showMenu.value
    }

    function remove(idx: number) {
        attachments.value.splice(idx, 1)
    }

    function clear() {
        attachments.value = []
    }

    /** 判断某类型在当前 capabilities 下是否可用 */
    function isTypeAvailable(type: string, caps: { mmproj_loaded: boolean; image_input: boolean; audio_input: boolean; video_input: boolean; text_input: boolean }): boolean {
        if (type === 'image') return caps.mmproj_loaded && caps.image_input
        if (type === 'audio') return caps.mmproj_loaded && caps.audio_input
        if (type === 'video') return caps.mmproj_loaded && caps.video_input
        if (type === 'text' || type === 'pdf') return caps.text_input
        return false
    }

    function processImage(file: File) {
        if (!file.type.startsWith('image/')) return
        if (!canAddImage.value) return
        const reader = new FileReader()
        reader.onload = () => {
            attachments.value.push({
                type: 'image',
                name: file.name,
                mime_type: file.type,
                data: reader.result as string,
            })
        }
        reader.readAsDataURL(file)
    }

    function processAudio(file: File) {
        const ext = file.name.split('.').pop()?.toLowerCase() || 'wav'
        const reader = new FileReader()
        reader.onload = () => {
            const base64 = (reader.result as string).split(',')[1]
            attachments.value.push({
                type: 'audio',
                name: file.name,
                mime_type: file.type || `audio/${ext}`,
                data: base64,
                format: ext,
            })
        }
        reader.readAsDataURL(file)
    }

    function processPdf(file: File) {
        const reader = new FileReader()
        reader.onload = () => {
            const base64 = (reader.result as string).split(',')[1]
            attachments.value.push({
                type: 'pdf',
                name: file.name,
                mime_type: 'application/pdf',
                data: base64,
            })
        }
        reader.readAsDataURL(file)
    }

    function processVideo(file: File) {
        const reader = new FileReader()
        reader.onload = () => {
            const base64 = (reader.result as string).split(',')[1]
            attachments.value.push({
                type: 'video',
                name: file.name,
                mime_type: file.type || 'video/mp4',
                data: base64,
            })
        }
        reader.readAsDataURL(file)
    }

    function processText(file: File) {
        const reader = new FileReader()
        reader.onload = () => {
            attachments.value.push({
                type: 'text',
                name: file.name,
                mime_type: file.type || 'text/plain',
                data: reader.result as string,
            })
        }
        reader.readAsText(file)
    }

    function processFiles(files: FileList | File[], type: string) {
        for (const file of Array.from(files)) {
            if (type === 'image') processImage(file)
            else if (type === 'audio') processAudio(file)
            else if (type === 'pdf') processPdf(file)
            else if (type === 'video') processVideo(file)
            else processText(file)
        }
    }

    /** 点击 outside 关闭菜单 */
    function onClickOutside(e: MouseEvent, containerSelector = '.attach-wrapper') {
        const target = e.target as HTMLElement
        if (!target.closest(containerSelector)) {
            closeMenu()
        }
    }

    return {
        attachments,
        showMenu,
        pendingType,
        imageCount,
        canAddImage,
        toggleMenu,
        closeMenu,
        remove,
        clear,
        processFiles,
        isTypeAvailable,
        onClickOutside,
        getAcceptForType,
        getTypeLabel,
    }
}
