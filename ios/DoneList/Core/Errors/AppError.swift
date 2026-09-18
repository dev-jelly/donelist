import Foundation

enum AppError: LocalizedError, Equatable {
    case networkError(String)
    case invalidResponse
    case unauthorized
    case forbidden
    case notFound
    case serverError(String)
    case decodingError(String)
    case encodingError
    case validationError(String)
    case authenticationError(String)
    case tokenExpired
    case syncError(String)
    case storageError(String)
    case unknown(String)

    var errorDescription: String? {
        switch self {
        case .networkError(let message): return "네트워크 오류: \(message)"
        case .invalidResponse: return "잘못된 응답입니다"
        case .unauthorized: return "인증이 필요합니다"
        case .forbidden: return "접근 권한이 없습니다"
        case .notFound: return "요청한 리소스를 찾을 수 없습니다"
        case .serverError(let message): return "서버 오류: \(message)"
        case .decodingError(let message): return "데이터 파싱 오류: \(message)"
        case .encodingError: return "데이터 인코딩 오류"
        case .validationError(let message): return message
        case .authenticationError(let message): return message
        case .tokenExpired: return "세션이 만료되었습니다. 다시 로그인해주세요"
        case .syncError(let message): return "동기화 오류: \(message)"
        case .storageError(let message): return "저장소 오류: \(message)"
        case .unknown(let message): return message.isEmpty ? "알 수 없는 오류가 발생했습니다" : message
        }
    }

    static func == (lhs: AppError, rhs: AppError) -> Bool {
        lhs.errorDescription == rhs.errorDescription
    }
}
