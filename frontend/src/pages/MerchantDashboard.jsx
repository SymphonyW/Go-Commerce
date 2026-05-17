import { useEffect, useMemo, useState } from 'react';
import { Link, useNavigate, useSearchParams } from 'react-router-dom';
import EmptyState from '../components/EmptyState';
import LoadingState from '../components/LoadingState';
import MerchantConsoleNav from '../components/MerchantConsoleNav';
import StatCard from '../components/StatCard';
import StatusBadge from '../components/StatusBadge';
import { merchantAPI } from '../services/api';
import { formatCurrency, formatDateTime, getOrderStatusLabel } from '../utils/display';

const MerchantDashboard = () => {
  const navigate = useNavigate();
  const [searchParams] = useSearchParams();
  const merchantId = searchParams.get('merchant_id') || undefined;
  const [profile, setProfile] = useState(null);
  const [products, setProducts] = useState([]);
  const [orders, setOrders] = useState([]);
  const [orderTotal, setOrderTotal] = useState(0);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');

  useEffect(() => {
    const loadDashboard = async () => {
      const token = localStorage.getItem('token');
      const role = localStorage.getItem('role');
      if (!token) {
        navigate('/login');
        return;
      }
      if (role !== 'merchant' && role !== 'admin') {
        navigate('/');
        return;
      }

      try {
        setLoading(true);
        setError('');
        const params = merchantId ? { merchant_id: merchantId } : {};
        const [profileResp, productResp, orderResp] = await Promise.all([
          merchantAPI.getProfile(params),
          merchantAPI.listConsoleProducts({ ...params, page: 1, page_size: 100 }),
          merchantAPI.listConsoleOrders({ ...params, page: 1, page_size: 100 }),
        ]);
        setProfile(profileResp.merchant);
        setProducts(productResp.products || []);
        setOrders(orderResp.orders || []);
        setOrderTotal(orderResp.total || 0);
      } catch (loadError) {
        setError(loadError.response?.data?.error || '加载商家后台失败');
      } finally {
        setLoading(false);
      }
    };

    loadDashboard();
  }, [merchantId, navigate]);

  const stats = useMemo(() => {
    const activeProducts = products.filter((item) => item.stock > 0).length;
    const pendingOrders = orders.filter((item) => item.status === 'pending' || item.status === 'paid').length;
    return {
      productTotal: products.length,
      activeProducts,
      orderTotal,
      pendingOrders,
    };
  }, [orderTotal, orders, products]);

  if (loading) {
    return <LoadingState label="正在加载商家后台..." />;
  }

  return (
    <div className="merchant-console">
      <MerchantConsoleNav />

      <section className="merchant-hero">
        <div>
          <p className="eyebrow">商家后台</p>
          <h1>{profile?.name || '尚未找到店铺'}</h1>
          <p>{profile ? `联系方式：${profile.contact_info}` : '先创建店铺，再开始管理商品和订单。'}</p>
        </div>
        <div className="merchant-hero-actions">
          {!profile ? (
            <Link to="/merchants/create" className="btn btn-primary">
              创建店铺
            </Link>
          ) : (
            <>
              <Link to={`/merchant/products${merchantId ? `?merchant_id=${merchantId}` : ''}`} className="btn btn-primary">
                管理商品
              </Link>
              <Link to={`/merchant/orders${merchantId ? `?merchant_id=${merchantId}` : ''}`} className="btn btn-secondary">
                查看订单
              </Link>
            </>
          )}
        </div>
      </section>

      {error && <div className="error-message">{error}</div>}

      {profile && (
        <>
          <section className="stat-grid">
            <StatCard label="商品总数" value={stats.productTotal} helper="当前店铺全部商品" icon="□" />
            <StatCard label="当前在售" value={stats.activeProducts} helper="库存大于 0" tone="success" icon="✓" />
            <StatCard label="订单总数" value={stats.orderTotal} helper="历史相关订单" tone="accent" icon="◎" />
            <StatCard label="待处理订单" value={stats.pendingOrders} helper="待支付 / 已支付" tone="warning" icon="!" />
          </section>

          <section className="merchant-section">
            <div className="section-heading">
              <div>
                <h2>最近订单</h2>
                <p>优先查看刚进入履约链路的订单。</p>
              </div>
              <Link to={`/merchant/orders${merchantId ? `?merchant_id=${merchantId}` : ''}`} className="btn btn-secondary btn-sm">
                查看全部
              </Link>
            </div>

            {orders.length === 0 ? (
              <EmptyState compact title="暂无相关订单" description="有订单进入店铺后，这里会先展示最近几笔。" />
            ) : (
              <div className="merchant-table">
                <div className="merchant-table-head">
                  <span>订单号</span>
                  <span>状态</span>
                  <span>金额</span>
                  <span>下单时间</span>
                </div>
                {orders.slice(0, 5).map((order) => (
                  <div key={order.id} className="merchant-table-row">
                    <span>#{order.id}</span>
                    <StatusBadge status={order.status} label={getOrderStatusLabel(order.status, order.cancel_reason)} />
                    <span>{formatCurrency(order.total_amount)}</span>
                    <span>{formatDateTime(order.created_at)}</span>
                  </div>
                ))}
              </div>
            )}
          </section>
        </>
      )}
    </div>
  );
};

export default MerchantDashboard;
