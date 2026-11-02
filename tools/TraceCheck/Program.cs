using System.Buffers.Binary;
using System.Net;
using System.Text.Json;
using Packet.Ax25;
using Packet.Core;

// Parse raw captures with the audited upstream AX.25 parser. This is a report
// adapter, not a second protocol implementation. IPv4 fields are inspected only.
foreach (var line in File.ReadLines(args[0]))
{
    if (string.IsNullOrWhiteSpace(line)) continue;
    using var doc = JsonDocument.Parse(line);
    var root = doc.RootElement;
    byte[] raw = Convert.FromHexString(root.GetProperty("frame").GetString()!);
    if (!Ax25Frame.TryParse(raw, Ax25ParseOptions.Strict, out var frame))
        throw new InvalidDataException("AX.25 frame rejected by upstream strict parser");
    if (!frame.IsUi || frame.Pid != 0xCC) continue;
    var ip = frame.Info.Span;
    if (ip.Length < 20 || ip[0] >> 4 != 4 || BinaryPrimitives.ReadUInt16BigEndian(ip[2..]) != ip.Length)
        throw new InvalidDataException("Invalid IPv4 header/length in UI/PID 0xCC");
    Console.WriteLine(JsonSerializer.Serialize(new
    {
        direction = root.GetProperty("dir").GetString(),
        source = frame.Source.Callsign.ToString(),
        destination = frame.Destination.Callsign.ToString(),
        pid = frame.Pid,
        sourceIp = new IPAddress(ip.Slice(12, 4)).ToString(),
        destinationIp = new IPAddress(ip.Slice(16, 4)).ToString(),
        protocol = ip[9],
        ipHex = Convert.ToHexString(ip),
        length = ip.Length
    }));
}
