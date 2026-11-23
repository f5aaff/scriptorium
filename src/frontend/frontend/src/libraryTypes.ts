/** LibraryItem is public catalogue metadata; extracted text is never included. */
export interface LibraryItem {
  Title: string;
  Author: string;
  PublishDate: string;
  LastUpdated: string;
  FileType: string;
  DocType: string;
  DeweyDecimal: string;
  Path: string;
  Uuid: string;
  OriginalFilename?: string;
  MediaType?: string;
  Size?: number;
  SHA256?: string;
  ExtractionStatus?: string;
  ClassificationStatus?: string;
  SuggestedDeweyDecimal?: string;
  SuggestedDeweyName?: string;
  ClassificationConfidence?: number;
}
