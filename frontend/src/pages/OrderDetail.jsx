import { useEffect, useState } from 'react';
import { Link, useNavigate, useParams } from 'react-router-dom';
import ErrorState from '../components/ErrorState';
import LoadingState from '../components/LoadingState';
import PageHeader from '../components/PageHeader';
import SectionCard from '../components/SectionCard';
import StatusBadge from '../components/StatusBadge';
import { orderAPI, paymentAPI } from '../services/api';
import { formatMoney, formatDateTime, getOrderStatusLabel } from '../utils/display';

const OrderDetail = () => {
  const { id } = useParams();
  const navigate = useNavigate();
  const [order, setOrder] = useState(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [feedback, setFeedback] = useState(null);
  const [cancelling, setCancelling] = useState(false);
  const [payment, setPayment] = useState(null);
  const [paying, setPaying] = useState(false);
  const [completing, setCompleting] = useState(false);

  useEffect(() => {
    const fetchOrder = async () => {
      const token = localStorage.getItem('token');
      if (!token) {
        navigate('/login');
        return;
      }

      try {
        const data = await orderAPI.getOrder(id);
        setOrder(data.order);
      } catch (fetchError) {
        console.error('Failed to fetch order:', fetchError);
        setError('获取订单详情失败');
      } finally {
        setLoading(false);
      }
    };

    fetchOrder();
  }, [id, navigate]);

  const refreshOrder = async () => {
    const updatedOrder = await orderAPI.getOrder(id);
    setOrder(updatedOrder.order);
  };

  const handleCancelOrder = async () => {
    if (!window.confirm('确定要取消这个订单吗？')) return;

    try {
      setCancelling(true);
      const response = await orderAPI.cancelOrder(id);
      if (response.success) {
        await refreshOrder();
        setFeedback({ type: 'success', text: '订单已取消。' });
      } else {
        setFeedback({ type: 'error', text: response.message || '取消订单失败。' });
      }
    } catch (actionError) {
      console.error('取消订单失败:', actionError);
      setFeedback({ type: 'error', text: '取消订单失败，请稍后重试。' });
    } finally {
      setCancelling(false);
    }
  };

  const handleCreatePayment = async () => {
    try {
      setPaying(true);
      const response = await paymentAPI.createPayment({
        order_id: Number(id),
        payment_method: 'mock_balance',
      });
      setPayment(response.payment);
      setFeedback({ type: 'success', text: '支付记录已创建。' });
    } catch (actionError) {
      console.error('创建支付失败:', actionError);
      setFeedback({ type: 'error', text: actionError.response?.data?.error || '创建支付失败。' });
    } finally {
      setPaying(false);
    }
  };

  const handlePaymentSuccess = async () => {
    if (!payment) return;

    try {
      setPaying(true);
      const response = await paymentAPI.markSuccess(payment.id);
      setPayment(response.payment);
      await refreshOrder();
      setFeedback({ type: 'success', text: '支付成功。' });
    } catch (actionError) {
      console.error('模拟支付成功失败:', actionError);
      setFeedback({ type: 'error', text: actionError.response?.data?.error || '支付失败。' });
    } finally {
      setPaying(false);
    }
  };

  const handlePaymentFail = async () => {
    if (!payment) return;

    try {
      setPaying(true);
      const response = await paymentAPI.markFailed(payment.id);
      setPayment(response.payment);
      setFeedback({ type: 'info', text: '已模拟支付失败，订单仍保持待支付。' });
    } catch (actionError) {
      console.error('模拟支付失败操作异常:', actionError);
      setFeedback({ type: 'error', text: actionError.response?.data?.error || '支付失败操作异常。' });
    } finally {
      setPaying(false);
    }
  };

  const handleCompleteOrder = async () => {
    if (!window.confirm('确认已经收到商品了吗？')) return;

    try {
      setCompleting(true);
      await orderAPI.completeOrder(id);
      await refreshOrder();
      setFeedback({ type: 'success', text: '已确认收货。' });
    } catch (actionError) {
      console.error('确认收货失败:', actionError);
      setFeedback({ type: 'error', text: actionError.response?.data?.error || '确认收货失败。' });
    } finally {
      setCompleting(false);
    }
  };

  if (loading) {
    return <LoadingState label="正在加载订单详情..." />;
  }

  if (error || !order) {
    return <ErrorState title="订单暂时不可用" description={error || '订单不存在'} />;
  }

  return (
    <div className="order-detail">
      <PageHeader
        eyebrow="Order Detail"
        title={`订单 #${order.id}`}
        subtitle="把状态、金额、商品明细和后续动作放在同一张交易单据里。"
        actions={
          <Link to="/orders" className="btn btn-secondary btn-sm">
            返回订单列表
          </Link>
        }
      />

      {feedback && <div className={`notice notice-${feedback.type}`}>{feedback.text}</div>}

      <SectionCard className="order-detail-card">
        <div className="order-detail-header">
          <div>
            <p>当前状态</p>
            <StatusBadge status={order.status} label={getOrderStatusLabel(order.status, order.cancel_reason)} />
          </div>
          <div className="order-detail-status-container">
            {order.status === 'pending' && (
              <button type="button" onClick={handleCancelOrder} disabled={cancelling} className="btn btn-danger btn-sm">
                {cancelling ? '取消中...' : '取消订单'}
              </button>
            )}
            {order.status === 'pending' && !payment && (
              <button type="button" onClick={handleCreatePayment} disabled={paying} className="btn btn-primary btn-sm">
                {paying ? '创建支付中...' : '去支付'}
              </button>
            )}
            {order.status === 'shipped' && (
              <button type="button" onClick={handleCompleteOrder} disabled={completing} className="btn btn-primary btn-sm">
                {completing ? '确认中...' : '确认收货'}
              </button>
            )}
          </div>
        </div>

        <div className="order-detail-summary">
          <div>
            <p>下单时间</p>
            <strong>{formatDateTime(order.created_at)}</strong>
          </div>
          <div>
            <p>订单总额</p>
            <strong>{formatMoney(order.total_amount_cents)}</strong>
          </div>
        </div>

        {payment && order.status === 'pending' && (
          <div className="payment-actions">
            <h3>支付模拟</h3>
            <div>支付单号：{payment.payment_no}</div>
            <div>支付状态：{payment.status}</div>
            {payment.status === 'created' && (
              <div className="payment-buttons">
                <button type="button" onClick={handlePaymentSuccess} disabled={paying} className="btn btn-primary btn-sm">
                  模拟支付成功
                </button>
                <button type="button" onClick={handlePaymentFail} disabled={paying} className="btn btn-danger btn-sm">
                  模拟支付失败
                </button>
              </div>
            )}
            {payment.status === 'failed' && (
              <div className="payment-buttons">
                <button type="button" onClick={handleCreatePayment} disabled={paying} className="btn btn-secondary btn-sm">
                  {paying ? '重新创建中...' : '重新发起支付'}
                </button>
              </div>
            )}
          </div>
        )}

        {order.status === 'paid' && <div className="payment-actions">订单已支付，等待商家发货。</div>}
        {order.status === 'completed' && <div className="payment-actions">订单已完成。</div>}
        {order.status === 'cancelled' && order.cancel_reason === 'payment_timeout' && (
          <div className="payment-actions">该订单因支付超时已自动取消。</div>
        )}

        <div>
          <div className="section-card-header">
            <div>
              <h2>订单商品</h2>
              <p>每一项都保留了下单时的价格快照。</p>
            </div>
          </div>
          <div className="order-items-list">
            {order.items.map((item, index) => (
              <div key={index} className="order-item">
                <div className="order-item-name">{item.product_name}</div>
                <div>数量：{item.quantity}</div>
                <div>单价：{formatMoney(item.price_cents)}</div>
                <div>小计：{formatMoney(item.price_cents * item.quantity)}</div>
              </div>
            ))}
          </div>
        </div>
      </SectionCard>
    </div>
  );
};

export default OrderDetail;
