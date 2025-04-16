export abstract class DeleteSegmentRepository {
  abstract delete: (
    organizationId: string,
    ledgerId: string,
    segmentId: string
  ) => Promise<void>
}
