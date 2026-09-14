import AppKit
import Foundation

enum PasteImageError: LocalizedError {
    case noImage
    case cannotEncodeImage

    var errorDescription: String? {
        switch self {
        case .noImage:
            return "剪贴板中没有图片，或没有可读取的图片文件。"
        case .cannotEncodeImage:
            return "剪贴板中的内容无法转换为 PNG。"
        }
    }
}

let pasteboard = NSPasteboard.general

func pngData(from data: Data) -> Data? {
    guard let bitmap = NSBitmapImageRep(data: data) else {
        return nil
    }
    return bitmap.representation(using: .png, properties: [:])
}

func clipboardImageData() throws -> Data {
    if let png = pasteboard.data(forType: .png) {
        return png
    }

    if let tiff = pasteboard.data(forType: .tiff), let png = pngData(from: tiff) {
        return png
    }

    if let urls = pasteboard.readObjects(forClasses: [NSURL.self], options: nil) as? [NSURL],
       let fileURL = urls.compactMap({ $0.filePathURL }).first,
       fileURL.isFileURL,
       FileManager.default.isReadableFile(atPath: fileURL.path),
       let fileData = try? Data(contentsOf: fileURL),
       let png = pngData(from: fileData) {
        return png
    }

    throw PasteImageError.noImage
}

do {
    let imageData = try clipboardImageData()
    let fileManager = FileManager.default
    let imageDirectory = fileManager.temporaryDirectory
        .appendingPathComponent("ai-image-paste", isDirectory: true)
    try fileManager.createDirectory(
        at: imageDirectory,
        withIntermediateDirectories: true,
        attributes: [.posixPermissions: 0o700]
    )

    let imageURL = imageDirectory
        .appendingPathComponent("image-\(UUID().uuidString).png")
    try imageData.write(to: imageURL, options: .atomic)
    try fileManager.setAttributes(
        [.posixPermissions: 0o600],
        ofItemAtPath: imageURL.path
    )

    // iTerm2 Run Coprocess injects stdout into the active terminal session.
    FileHandle.standardOutput.write(Data(imageURL.path.utf8))
} catch {
    let message = error.localizedDescription + "\n"
    FileHandle.standardError.write(Data(message.utf8))
    exit(1)
}
