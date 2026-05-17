import { useEffect, useMemo, useState } from 'react';
import { Link, useNavigate, useSearchParams } from 'react-router-dom';
import MerchantConsoleNav from '../components/MerchantConsoleNav';
import { merchantAPI } from '../services/api';

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
      } catch (err) {
        setError(err.response?.data?.error || '加载商家后台失败');
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
    return <div className="loading">加载中...</div>;
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
        {!profile && (
          <Link to="/merchants/create" className="btn btn-primary">
            创建店铺
          </Link>
        )}
      </section>

      {error && <div className="error-message">{error}</div>}

      {profile && (
        <>
          <section className="merchant-metrics">
            <div>
              <span>商品总数</span>
              <strong>{stats.productTotal}</strong>
            </div>
            <div>
              <span>当前在售</span>
              <strong>{stats.activeProducts}</strong>
            </div>
            <div>
              <span>订单总数</span>
              <strong>{stats.orderTotal}</strong>
            </div>
            <div>
              <span>待处理订单</span>
              <strong>{stats.pendingOrders}</strong>
            </div>
          </section>

          <section className="merchant-section">
            <div className="section-heading">
              <div>
                <h2>最近订单</h2>
                <p>优先查看刚进入履约链路的订单。</p>
              </div>
              <Link to={`/merchant/orders${merchantId ? `?merchant_id=${merchantId}` : ''}`} className="btn btn-secondary">
                查看全部
              </Link>
            </div>

            {orders.length === 0 ? (
              <div className="empty-state compact">暂无相关订单</div>
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
                    <span className={`status-pill ${order.status}`}>{order.status}</span>
                    <span>¥{Number(order.total_amount || 0).toFixed(2)}</span>
                    <span>{new Date(order.created_at).toLocaleString()}</span>
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
