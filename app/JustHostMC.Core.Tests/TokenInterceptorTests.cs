using System.Text;
using Grpc.Core;
using Grpc.Core.Interceptors;
using JustHostMC.Core;
using Xunit;

namespace JustHostMC.Core.Tests;

public class TokenInterceptorTests
{
    private static readonly Marshaller<string> StringMarshaller =
        Marshallers.Create(Encoding.UTF8.GetBytes, Encoding.UTF8.GetString);

    private static readonly Method<string, string> UnaryMethod =
        new(MethodType.Unary, "test.Service", "Method", StringMarshaller, StringMarshaller);

    [Fact]
    public void AsyncUnaryCall_AttachesTokenHeader()
    {
        var interceptor = new TokenInterceptor("the-token");
        var context = new ClientInterceptorContext<string, string>(UnaryMethod, host: null, new CallOptions());
        Metadata? captured = null;

        interceptor.AsyncUnaryCall("req", context, (request, ctx) =>
        {
            captured = ctx.Options.Headers;
            return new AsyncUnaryCall<string>(
                Task.FromResult("resp"),
                Task.FromResult(new Metadata()),
                () => Status.DefaultSuccess,
                () => new Metadata(),
                () => { });
        });

        Assert.NotNull(captured);
        Assert.Equal("the-token", captured!.GetValue(TokenInterceptor.TokenHeader));
    }

    [Fact]
    public void AsyncUnaryCall_DoesNotDuplicateExistingHeader()
    {
        var interceptor = new TokenInterceptor("the-token");
        var preset = new Metadata { { TokenInterceptor.TokenHeader, "already-set" } };
        var context = new ClientInterceptorContext<string, string>(
            UnaryMethod, host: null, new CallOptions(headers: preset));
        Metadata? captured = null;

        interceptor.AsyncUnaryCall("req", context, (request, ctx) =>
        {
            captured = ctx.Options.Headers;
            return new AsyncUnaryCall<string>(
                Task.FromResult("resp"),
                Task.FromResult(new Metadata()),
                () => Status.DefaultSuccess,
                () => new Metadata(),
                () => { });
        });

        Assert.NotNull(captured);
        Assert.Single(captured!, e => e.Key == TokenInterceptor.TokenHeader);
        Assert.Equal("already-set", captured!.GetValue(TokenInterceptor.TokenHeader));
    }
}
