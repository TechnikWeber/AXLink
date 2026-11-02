using Packet.Net.Bridge;
using Packet.Net.Arp;
using Packet.Net.Rhp;
using Packet.Net.Routing;
using Packet.Net.Tests.Fakes;
using Xunit;

namespace Packet.Net.Tests;

// Additive regression tests for pdn-net commit 91c412c91604c17bc28abd49e66443377292e5dc.
// Uses upstream fakes and bridge; no KISS, AX.25 or IP stack implementation.
public class AXLinkDestinationFilterTests
{
    [Theory]
    [InlineData("TESTB-1", true)]
    [InlineData("TESTC-1", false)]
    [InlineData("TESTB-2", false)]
    [InlineData("testb-1", true)]
    [InlineData("QST", false)]
    public async Task Only_the_addressed_station_receives_unicast_ip(string destination, bool deliver)
    {
        using var tun = new FakeTunDevice();
        await using var rhp = new FakeRhpCustomClient();
        var resolver = CallsignResolver.FromRoutes(Array.Empty<RouteEntry>());
        var bridge = new IpAx25Bridge(tun, rhp, resolver, myCallsign: "TESTB-1");

        // IPv4/UDP, 10.73.0.1 -> 10.73.0.2, ports 12345 -> 12346, payload "AXL1".
        // Valid IPv4 header checksum; UDP checksum zero is permitted for IPv4.
        byte[] packet = Convert.FromHexString(
            "4500002000010000401166380A4900010A4900023039303A000C000041584C31");
        var data = new byte[packet.Length + 1];
        data[0] = 0xCC;
        packet.CopyTo(data, 1);

        await bridge.HandleInboundAsync(
            new RhpDatagram("TESTA-1", destination, data), CancellationToken.None);

        if (deliver)
            Assert.Equal(packet, Assert.Single(tun.Written));
        else
            Assert.Empty(tun.Written);
        Assert.Empty(rhp.Sends);
    }

    [Theory]
    [InlineData("TESTB", "TESTB-0")]
    [InlineData("TESTB-0", "testb")]
    public async Task Zero_ssid_has_the_same_identity(string local, string destination)
    {
        using var tun = new FakeTunDevice();
        await using var rhp = new FakeRhpCustomClient();
        var bridge = new IpAx25Bridge(tun, rhp,
            CallsignResolver.FromRoutes(Array.Empty<RouteEntry>()), myCallsign: local);
        await bridge.HandleInboundAsync(new RhpDatagram("TESTA-1", destination,
            new byte[] { 0xCC, 0x45 }), CancellationToken.None);
        Assert.Single(tun.Written);
    }

    [Fact]
    public async Task Foreign_segments_cannot_complete_or_poison_a_local_reassembly()
    {
        using var tun = new FakeTunDevice();
        await using var rhp = new FakeRhpCustomClient();
        var bridge = new IpAx25Bridge(tun, rhp,
            CallsignResolver.FromRoutes(Array.Empty<RouteEntry>()), myCallsign: "TESTB-1");
        // Exercise the upstream segmentation format; not an interoperability claim.
        await bridge.HandleInboundAsync(new RhpDatagram("TESTA-1", "TESTB-1",
            new byte[] { 0x08, 0x00, 0x45 }), CancellationToken.None);
        await bridge.HandleInboundAsync(new RhpDatagram("TESTA-1", "TESTC-1",
            new byte[] { 0x08, 0x03, 0xFF }), CancellationToken.None);
        Assert.Empty(tun.Written);
        await bridge.HandleInboundAsync(new RhpDatagram("TESTA-1", "TESTB-1",
            new byte[] { 0x08, 0x03, 0x00 }), CancellationToken.None);
        Assert.Equal(new byte[] { 0x45, 0x00 }, Assert.Single(tun.Written));
    }

    [Fact]
    public async Task Broadcast_arp_for_our_ip_still_receives_a_reply()
    {
        using var tun = new FakeTunDevice();
        await using var rhp = new FakeRhpCustomClient();
        byte[] localIp = { 10, 73, 0, 2 };
        byte[] remoteIp = { 10, 73, 0, 1 };
        var cache = new ArpCache();
        var bridge = new IpAx25Bridge(tun, rhp,
            CallsignResolver.FromRoutes(Array.Empty<RouteEntry>()),
            myCallsign: "TESTB-1", myIp: localIp, arpCache: cache);
        byte[] arp = ArpPacket.BuildRequest("TESTA-1", remoteIp, localIp);
        var data = new byte[arp.Length + 1];
        data[0] = 0xCD;
        arp.CopyTo(data, 1);
        await bridge.HandleInboundAsync(new RhpDatagram("TESTA-1", "QST", data),
            CancellationToken.None);
        var reply = Assert.Single(rhp.Sends);
        Assert.Equal("TESTA-1", reply.DestCallsign);
        Assert.Equal(0xCD, reply.Data[0]);
        Assert.True(cache.TryResolve(remoteIp, out var learned));
        Assert.Equal("TESTA-1", learned);
        Assert.Empty(tun.Written);
    }
}
